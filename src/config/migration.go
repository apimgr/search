package config

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/apimgr/search/src/database"
	"gopkg.in/yaml.v3"
)

const serverConfigHeader = "Settings of this node only (listen, identity, database, cache, ssl). Everything else lives in yaml.d/: branding.yml, web.yml, pages.yml, contact.yml, privacy.yml, security.yml, logs.yml, metrics.yml, notifications.yml, tor.yml, i2p.yml, maintenance.yml, accounts.yml, app.yml. Credentials live in secrets.yml."

var areaFiles = map[string]string{
	"branding":      "branding.yml",
	"web":           "web.yml",
	"pages":         "pages.yml",
	"contact":       "contact.yml",
	"privacy":       "privacy.yml",
	"security":      "security.yml",
	"logs":          "logs.yml",
	"metrics":       "metrics.yml",
	"notifications": "notifications.yml",
	"tor":           "tor.yml",
	"i2p":           "i2p.yml",
	"maintenance":   "maintenance.yml",
	"accounts":      "accounts.yml",
	"app":           "app.yml",
}

var areaFileOrder = []string{
	"branding", "web", "pages", "contact", "privacy", "security", "logs", "metrics",
	"notifications", "tor", "i2p", "maintenance", "accounts", "app",
}

var serverNodeKeys = map[string]bool{
	"port": true, "address": true, "fqdn": true, "mode": true,
	"api_version": true, "baseurl": true, "user": true, "group": true,
	"pidfile": true, "daemonize": true, "debug": true, "database": true,
	"cache": true, "ssl": true, "trusted_proxies": true,
}

func partitionConfigNode(document *yaml.Node) (map[string]*yaml.Node, *yaml.Node) {
	areas := make(map[string]*yaml.Node, len(areaFiles))
	for area := range areaFiles {
		areas[area] = emptyYAMLDocument()
	}
	root := configYAMLRoot(document)
	if root == nil || root.Kind != yaml.MappingNode {
		return areas, document
	}
	server := mappingValue(root, "server")
	if server != nil && server.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(server.Content); {
			key, value := server.Content[index], server.Content[index+1]
			if serverNodeKeys[key.Value] {
				index += 2
				continue
			}
			area := serverArea(key.Value)
			target := areas[area]
			appendConfigChild(target, []string{"server", key.Value}, value)
			server.Content = append(server.Content[:index], server.Content[index+2:]...)
		}
	}
	for index := 0; index+1 < len(root.Content); {
		key, value := root.Content[index], root.Content[index+1]
		switch key.Value {
		case "server":
			index += 2
		case "web":
			if value.Kind == yaml.MappingNode {
				for webIndex := 0; webIndex+1 < len(value.Content); {
					webKey, webValue := value.Content[webIndex], value.Content[webIndex+1]
					area := "web"
					if webKey.Value == "ui" {
						area = "branding"
					}
					appendConfigChild(areas[area], []string{"web", webKey.Value}, webValue)
					value.Content = append(value.Content[:webIndex], value.Content[webIndex+2:]...)
				}
			}
			root.Content = append(root.Content[:index], root.Content[index+2:]...)
		case "tor":
			appendConfigChild(areas["tor"], []string{"tor"}, value)
			root.Content = append(root.Content[:index], root.Content[index+2:]...)
		case "data":
			appendConfigChild(areas["security"], []string{"data"}, value)
			root.Content = append(root.Content[:index], root.Content[index+2:]...)
		case "cli":
			appendConfigChild(areas["maintenance"], []string{"cli"}, value)
			root.Content = append(root.Content[:index], root.Content[index+2:]...)
		default:
			appendConfigChild(areas["app"], []string{key.Value}, value)
			root.Content = append(root.Content[:index], root.Content[index+2:]...)
		}
	}
	return areas, document
}

func serverArea(key string) string {
	switch key {
	case "branding", "seo":
		return "branding"
	case "cors", "csrf", "url_detection", "compression", "healthz", "i18n":
		return "web"
	case "pages":
		return "pages"
	case "contact":
		return "contact"
	case "privacy", "compliance", "tracking":
		return "privacy"
	case "security", "rate_limit", "geoip", "limits", "tokens":
		return "security"
	case "logs":
		return "logs"
	case "metrics":
		return "metrics"
	case "notifications":
		return "notifications"
	case "tor":
		return "tor"
	case "i2p":
		return "i2p"
	case "maintenance", "scheduler", "update", "auto_update", "backup", "restore":
		return "maintenance"
	case "auth", "users", "orgs", "session", "admin", "admin_path":
		return "accounts"
	default:
		return "app"
	}
}

func migrateConfigAreas(path string) (bool, error) {
	document, err := yamlNodeFromConfig(path)
	if err != nil {
		return false, err
	}
	original, err := yaml.Marshal(document)
	if err != nil {
		return false, err
	}
	changed := migrateLegacyKeys(document)
	changed = migrateLegacyHTTPSPort(document) || changed
	changed = migrateSchedulerTaskKeys(document) || changed
	secrets := emptyYAMLDocument()
	modifiedDropIns, movedSecrets, err := extractSecretsFromFiles(filepath.Dir(path), document, secrets)
	if err != nil {
		return false, err
	}
	changed = changed || movedSecrets
	areas, _ := partitionConfigNode(document)
	for _, area := range areas {
		if root := configYAMLRoot(area); root != nil && len(root.Content) > 0 {
			changed = true
		}
	}
	if changed {
		document.HeadComment = serverConfigHeader
	}
	serverBytes, err := yaml.Marshal(document)
	if err != nil {
		return false, err
	}
	if bytes.Equal(original, serverBytes) && !changed {
		return false, nil
	}
	if !changed && strings.TrimSpace(document.HeadComment) == strings.TrimSpace(serverConfigHeader) {
		return false, nil
	}
	if err := backupConfig(path); err != nil {
		slog.Warn("configuration area migration skipped; backup could not be written", "path", path, "err", err)
		return false, nil
	}
	dropInDir := filepath.Join(filepath.Dir(path), "yaml.d")
	for _, area := range areaFileOrder {
		filename := areaFiles[area]
		migrated := areas[area]
		root := configYAMLRoot(migrated)
		if root == nil || len(root.Content) == 0 {
			continue
		}
		areaPath := filepath.Join(dropInDir, filename)
		if existing, readErr := yamlNodeFromConfig(areaPath); readErr == nil {
			mergeConfigNodes(migrated, existing, "", areaPath, make(map[string]string))
		} else if !errors.Is(readErr, os.ErrNotExist) {
			slog.Warn("configuration area migration skipped; existing area file could not be read", "path", areaPath, "err", readErr)
			return false, nil
		}
		if err := writeYAMLAtomic(areaPath, migrated, ".area.yml-*"); err != nil {
			slog.Warn("configuration area migration skipped; area file could not be written", "path", areaPath, "err", err)
			return false, nil
		}
	}
	if hasConfigSecrets(secrets) {
		secretPath := filepath.Join(filepath.Dir(path), "secrets.yml")
		if existing, readErr := yamlNodeFromConfig(secretPath); readErr == nil {
			mergeConfigNodes(secrets, existing, "", secretPath, make(map[string]string))
		} else if !errors.Is(readErr, os.ErrNotExist) {
			slog.Warn("configuration area migration skipped; secrets file could not be read", "path", secretPath, "err", readErr)
			return false, nil
		}
		if err := writeYAMLAtomic(secretPath, secrets, ".secrets.yml-*"); err != nil {
			slog.Warn("configuration area migration skipped; secrets file could not be written", "path", secretPath, "err", err)
			return false, nil
		}
	}
	for dropInPath, dropInNode := range modifiedDropIns {
		if err := writeYAMLAtomic(dropInPath, dropInNode, ".drop-in.yml-*"); err != nil {
			slog.Warn("configuration area migration skipped; drop-in could not be rewritten", "path", dropInPath, "err", err)
			return false, nil
		}
	}
	document.HeadComment = serverConfigHeader
	if err := writeYAMLAtomic(path, document, ".server.yml-*"); err != nil {
		slog.Warn("configuration area migration skipped; server.yml could not be rewritten", "path", path, "err", err)
		return false, nil
	}
	return true, nil
}

func extractSecretsFromFiles(configDir string, serverDocument, secrets *yaml.Node) (map[string]*yaml.Node, bool, error) {
	modified := make(map[string]*yaml.Node)
	changed := false
	serverSecrets := emptyYAMLDocument()
	extractConfigSecrets(serverDocument, serverSecrets)
	if hasConfigSecrets(serverSecrets) {
		mergeConfigNodes(secrets, serverSecrets, "", "server.yml", make(map[string]string))
		changed = true
	}
	dropInPaths, err := configDropInPaths(filepath.Join(configDir, "server.yml"))
	if err != nil {
		return nil, false, fmt.Errorf("read config drop-in directory: %w", err)
	}
	for _, filePath := range dropInPaths {
		document, readErr := yamlNodeFromConfig(filePath)
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				continue
			}
			return nil, false, fmt.Errorf("read drop-in %s: %w", filePath, readErr)
		}
		before, _ := yaml.Marshal(document)
		legacyChanged := migrateLegacyKeys(document)
		legacyChanged = migrateLegacyHTTPSPort(document) || legacyChanged
		legacyChanged = migrateSchedulerTaskKeys(document) || legacyChanged
		if legacyChanged {
			changed = true
		}
		moved := emptyYAMLDocument()
		extractConfigSecrets(document, moved)
		if hasConfigSecrets(moved) {
			mergeConfigNodes(secrets, moved, "", filePath, make(map[string]string))
			changed = true
		}
		after, _ := yaml.Marshal(document)
		if !bytes.Equal(before, after) {
			modified[filePath] = document
		}
	}
	return modified, changed, nil
}

func migrateLegacyKeys(document *yaml.Node) bool {
	changed := false
	move := func(oldPath, newPath string) {
		if moveConfigPath(document, oldPath, newPath) {
			changed = true
		}
	}
	remove := func(path string) {
		if removeConfigPath(document, path) {
			changed = true
		}
	}
	remove("server.cluster")

	if logging := configPathValue(document, "server.logging"); logging != nil && logging.Kind == yaml.MappingNode {
		if accessFormat := mappingValue(logging, "access_format"); accessFormat != nil {
			moveConfigNode(document, "server.logs.access.format", accessFormat)
			removeMappingKey(logging, "access_format")
			changed = true
		}
		move("server.logging.level", "server.logs.level")
		move("server.logging", "server.logs")
	}
	if schedule := configPathValue(document, "server.schedule"); schedule != nil && schedule.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(schedule.Content); {
			if schedule.Content[index+1].Kind != yaml.MappingNode {
				schedule.Content = append(schedule.Content[:index], schedule.Content[index+2:]...)
				changed = true
				continue
			}
			index += 2
		}
		move("server.schedule", "server.scheduler")
	}
	if database := configPathValue(document, "database"); database != nil && database.Kind == yaml.MappingNode {
		if driver := mappingValue(database, "type"); driver != nil {
			moveConfigNode(document, "server.database.driver", driver)
			removeMappingKey(database, "type")
			changed = true
		}
		if primary := mappingValue(database, "primary"); primary != nil && primary.Kind == yaml.MappingNode {
			moveConfigNode(document, "server.database", primary)
			removeMappingKey(database, "primary")
			changed = true
		}
		move("database", "server.database")
	}
	move("server.web.cookie_consent.message", "server.privacy.consent.message")
	move("server.web.cookie_consent.policy_url", "server.privacy.consent.policy.url")
	remove("server.web.cookie_consent.enabled")
	remove("server.web.cookie_consent")
	move("server.web", "web")
	move("server.security_headers.hsts", "web.hsts.enabled")
	move("server.security_headers.hsts_max_age", "web.hsts.max_age_seconds")
	remove("server.security_headers")
	move("server.security.hsts", "web.hsts")
	move("server.security.nel", "web.headers.nel")
	move("server.security.headers.clear_site_data", "web.headers.clear_site_data")
	remove("server.security.headers")
	move("server.security.csrf", "server.csrf")
	move("web.csrf", "server.csrf")
	move("server.title", "server.branding.title")
	move("server.description", "server.branding.description")
	move("server.seo.default_title", "server.branding.title")
	move("server.seo.default_description", "server.branding.description")
	move("server.seo.opengraph.image", "server.seo.og_image")
	remove("server.seo.opengraph.description")
	move("server.seo.twitter.site", "server.seo.twitter_handle")
	remove("server.seo.enabled")
	remove("server.seo.sitemap.enabled")
	remove("server.seo.feed.enabled")
	move("server.branding.logo_url", "server.branding.logo")
	move("server.branding.favicon_url", "server.branding.favicon")
	move("server.branding.primary_color", "web.ui.accent")
	if footer := configPathValue(document, "server.branding.footer_text"); footer != nil {
		if migrated := legacyFooter(footer); migrated != nil {
			moveConfigNode(document, "server.branding.content.footer", migrated)
		}
		remove("server.branding.footer_text")
		changed = true
	}
	remove("server.branding.theme_color")
	remove("server.branding.source_code_url")
	move("web.favicon_url", "server.branding.favicon")
	move("web.ui.favicon", "server.branding.favicon")
	move("web.ui.logo", "server.branding.logo")
	if footer := configPathValue(document, "web.footer.custom_html"); footer != nil {
		body := cloneYAMLNode(footer)
		moveConfigNode(document, "server.branding.content.footer.format", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "html"})
		moveConfigNode(document, "server.branding.content.footer.body", body)
		remove("web.footer.custom_html")
		changed = true
	}
	for _, page := range []string{"about", "privacy", "terms"} {
		oldPath := "server.pages." + page + ".content"
		if body := configPathValue(document, oldPath); body != nil {
			moveConfigNode(document, "server.branding.content."+page+".format", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "md"})
			moveConfigNode(document, "server.branding.content."+page+".body", body)
			remove(oldPath)
			changed = true
		}
	}
	move("server.admin.email", "server.contact.admin.email")
	remove("server.admin.path")
	for _, oldPath := range []string{"server.admin.password", "server.admin.token", "server.admin.username", "server.admin.two_factor"} {
		remove(oldPath)
	}
	move("server.ssl.dns01.provider", "server.ssl.letsencrypt.dns_provider")
	move("server.ssl.dns01.credentials", "server.ssl.letsencrypt.dns_credentials")
	move("server.ssl.letsencrypt.provider", "server.ssl.letsencrypt.dns_provider")
	move("server.ssl.letsencrypt.credentials_encrypted", "server.ssl.letsencrypt.dns_credentials")
	for _, oldPath := range []string{
		"server.ssl.dns01.validated_at", "server.ssl.dns01.domain", "server.ssl.dns01.domains",
		"server.ssl.letsencrypt.validated_at", "server.ssl.letsencrypt.domain", "server.ssl.letsencrypt.domains",
	} {
		remove(oldPath)
	}
	for _, prefix := range []string{
		"server.privacy.content", "server.scheduler.enabled", "server.rate_limit.enabled", "server.rate_limit.requests", "server.rate_limit.window",
		"server.i18n.auto_detect", "server.i18n.rtl_languages", "server.i18n.show_selector",
		"server.i18n.translations_dir", "server.geoip.dir", "server.geoip.databases",
		"server.geoip.country_mode", "server.database.sqlite", "server.metrics.endpoint",
		"server.notifications.email_enabled", "server.notifications.webhook_enabled",
		"server.secret_key", "web.permissions_policy", "web.honor_dnt", "web.robots.ai_bots",
		"server.mcp", "server.maintenance_mode",
	} {
		remove(prefix)
	}
	return changed
}

func migrateSchedulerTaskKeys(document *yaml.Node) bool {
	tasks := configPathValue(document, "server.scheduler.tasks")
	if tasks == nil || tasks.Kind != yaml.MappingNode {
		return false
	}
	defaultDocument := &yaml.Node{}
	if err := defaultDocument.Encode(DefaultConfig()); err != nil {
		return false
	}
	defaultTasks := configPathValue(defaultDocument, "server.scheduler.tasks")
	changed := false
	for taskIndex := 0; taskIndex+1 < len(tasks.Content); taskIndex += 2 {
		taskName := tasks.Content[taskIndex].Value
		task := tasks.Content[taskIndex+1]
		if task.Kind != yaml.MappingNode {
			continue
		}
		allowed := map[string]bool{"enabled": true, "schedule": true, "retry_on_fail": true, "retry_delay": true}
		if taskName == "backup" || taskName == "backup_daily" || taskName == "backup_hourly" {
			allowed["retention"] = true
		}
		for keyIndex := 0; keyIndex+1 < len(task.Content); {
			if !allowed[task.Content[keyIndex].Value] {
				task.Content = append(task.Content[:keyIndex], task.Content[keyIndex+2:]...)
				changed = true
				continue
			}
			keyIndex += 2
		}
		if defaultTask := mappingValue(defaultTasks, taskName); defaultTask != nil && yamlNodesEqual(task, defaultTask) {
			tasks.Content = append(tasks.Content[:taskIndex], tasks.Content[taskIndex+2:]...)
			taskIndex -= 2
			changed = true
			continue
		}
		if len(task.Content) == 0 {
			tasks.Content = append(tasks.Content[:taskIndex], tasks.Content[taskIndex+2:]...)
			taskIndex -= 2
			changed = true
		}
	}
	return changed
}

func emptyYAMLDocument() *yaml.Node {
	return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
}

func appendConfigChild(document *yaml.Node, path []string, value *yaml.Node) {
	root := configYAMLRoot(document)
	current := root
	for _, part := range path[:len(path)-1] {
		next := mappingValue(current, part)
		if next == nil {
			next = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			current.Content = append(current.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: part, Tag: "!!str"}, next)
		}
		current = next
	}
	key := path[len(path)-1]
	if mappingValue(current, key) == nil {
		current.Content = append(current.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key, Tag: "!!str"}, cloneYAMLNode(value))
	}
}

func diffYAMLNode(value, defaults *yaml.Node) *yaml.Node {
	if value == nil {
		return nil
	}
	if value.Kind != yaml.MappingNode {
		if yamlNodesEqual(value, defaults) {
			return nil
		}
		return cloneYAMLNode(value)
	}
	result := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for index := 0; index+1 < len(value.Content); index += 2 {
		key, child := value.Content[index], value.Content[index+1]
		defaultChild := mappingValue(defaults, key.Value)
		changed := diffYAMLNode(child, defaultChild)
		if changed != nil {
			result.Content = append(result.Content, cloneYAMLNode(key), changed)
		}
	}
	if len(result.Content) == 0 {
		return nil
	}
	return result
}

func yamlNodesEqual(left, right *yaml.Node) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.Kind != right.Kind || left.Tag != right.Tag || left.Value != right.Value || len(left.Content) != len(right.Content) {
		return false
	}
	for index := range left.Content {
		if !yamlNodesEqual(left.Content[index], right.Content[index]) {
			return false
		}
	}
	return true
}

func saveAreaConfig(path string, current *yaml.Node) error {
	defaults := &yaml.Node{}
	if err := defaults.Encode(DefaultConfig()); err != nil {
		return err
	}
	defaultSecrets := emptyYAMLDocument()
	extractConfigSecrets(defaults, defaultSecrets)
	areas, _ := partitionConfigNode(current)
	defaultAreas, _ := partitionConfigNode(defaults)
	dropInDir := filepath.Join(filepath.Dir(path), "yaml.d")
	for _, area := range areaFileOrder {
		filename := areaFiles[area]
		areaNode := areas[area]
		root := configYAMLRoot(areaNode)
		if root == nil || len(root.Content) == 0 {
			continue
		}
		changed := diffYAMLNode(root, configYAMLRoot(defaultAreas[area]))
		areaPath := filepath.Join(dropInDir, filename)
		if info, err := os.Stat(areaPath); err == nil && info.Mode().IsRegular() {
			existing, readErr := yamlNodeFromConfig(areaPath)
			if readErr != nil {
				return fmt.Errorf("read area config %s: %w", areaPath, readErr)
			}
			if changed == nil {
				changed = existing
			} else {
				delta := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{changed}}
				mergeConfigNodes(existing, delta, "", areaPath, make(map[string]string))
				changed = existing
			}
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat area config %s: %w", areaPath, err)
		}
		changedRoot := configYAMLRoot(changed)
		if changed == nil || changedRoot == nil || len(changedRoot.Content) == 0 {
			continue
		}
		if err := writeYAMLAtomic(areaPath, changed, ".area.yml-*"); err != nil {
			return fmt.Errorf("write area config %s: %w", areaPath, err)
		}
	}
	return nil
}

func migrationBackupPath(path string) string { return path + ".bak" }

func backupConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	backup := migrationBackupPath(path)
	if err := os.WriteFile(backup, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(backup, 0o600)
}

func parsePortScalar(node *yaml.Node) (int, bool) {
	if node == nil || node.Kind != yaml.ScalarNode {
		return 0, false
	}
	port, err := strconv.Atoi(strings.TrimSpace(node.Value))
	return port, err == nil
}

func migrateLegacyHTTPSPort(document *yaml.Node) bool {
	server := mappingValue(configYAMLRoot(document), "server")
	legacy := mappingValue(server, "https_port")
	if legacy == nil {
		return false
	}
	port := mappingValue(server, "port")
	if port != nil && strings.Contains(port.Value, ",") {
		removeMappingKey(server, "https_port")
		return true
	}
	httpsPort, ok := parsePortScalar(legacy)
	if httpPort, single := parsePortScalar(port); ok && single && httpPort >= 0 && httpsPort > 0 && httpsPort <= 65535 {
		port.Value = strconv.Itoa(httpPort) + "," + strconv.Itoa(httpsPort)
		port.Tag = "!!str"
		removeMappingKey(server, "https_port")
		return true
	}
	removeMappingKey(server, "https_port")
	return true
}

func removeMappingKey(mapping *yaml.Node, key string) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			mapping.Content = append(mapping.Content[:index], mapping.Content[index+2:]...)
			return
		}
	}
}

func setConfigPath(document *yaml.Node, dottedPath string, value *yaml.Node) {
	parts := strings.Split(dottedPath, ".")
	appendConfigChild(document, parts, value)
}

func moveConfigNode(document *yaml.Node, dottedPath string, value *yaml.Node) {
	if value == nil {
		return
	}
	current := configPathValue(document, dottedPath)
	if current == nil {
		setConfigPath(document, dottedPath, value)
		return
	}
	if current.Kind == yaml.ScalarNode && strings.TrimSpace(current.Value) == "" && value.Kind == yaml.ScalarNode {
		*current = *cloneYAMLNode(value)
		return
	}
	mergeMissingNode(current, value)
}

func mergeMissingNode(destination, source *yaml.Node) {
	if destination == nil || source == nil || destination.Kind != yaml.MappingNode || source.Kind != yaml.MappingNode {
		return
	}
	for index := 0; index+1 < len(source.Content); index += 2 {
		key, value := source.Content[index], source.Content[index+1]
		current := mappingValue(destination, key.Value)
		if current == nil {
			destination.Content = append(destination.Content, cloneYAMLNode(key), cloneYAMLNode(value))
		} else if current.Kind == yaml.MappingNode && value.Kind == yaml.MappingNode {
			mergeMissingNode(current, value)
		}
	}
}

func moveConfigPath(document *yaml.Node, oldPath, newPath string) bool {
	value := configPathValue(document, oldPath)
	if value == nil {
		return false
	}
	moveConfigNode(document, newPath, value)
	removeConfigPath(document, oldPath)
	return true
}

func removeConfigPath(document *yaml.Node, dottedPath string) bool {
	parts := strings.Split(dottedPath, ".")
	root := configYAMLRoot(document)
	current := root
	parents := make([]*yaml.Node, 0, len(parts))
	keys := make([]string, 0, len(parts))
	for _, part := range parts[:len(parts)-1] {
		parents = append(parents, current)
		keys = append(keys, part)
		current = mappingValue(current, part)
		if current == nil || current.Kind != yaml.MappingNode {
			return false
		}
	}
	if mappingValue(current, parts[len(parts)-1]) == nil {
		return false
	}
	removeMappingKey(current, parts[len(parts)-1])
	for index := len(parents) - 1; index >= 0; index-- {
		child := mappingValue(parents[index], keys[index])
		if child == nil || child.Kind != yaml.MappingNode || len(child.Content) > 0 {
			break
		}
		removeMappingKey(parents[index], keys[index])
	}
	return true
}

func quotedString(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Style: yaml.DoubleQuotedStyle, Value: value}
}

func legacyFooter(value *yaml.Node) *yaml.Node {
	if value == nil || value.Kind != yaml.ScalarNode {
		return nil
	}
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "format"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "html"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "body"},
		quotedString("<p>" + html.EscapeString(value.Value) + "</p>"),
	}}
}

func (c *Config) ImportLegacyAppSecrets(ctx context.Context, db *database.DB) error {
	if c == nil || db == nil {
		return nil
	}
	secretNames := []string{
		"installation_secret",
		"installation_secret_previous",
		"installation_secret_previous_expires",
		"encryption_key_previous",
		"encryption_key_previous_expires",
	}
	for _, name := range secretNames {
		path := "server.security." + name
		databaseName := name
		if name == "encryption_key_previous_expires" {
			databaseName = database.SecretEncryptionKeyPreviousUntil
		}
		secret, err := database.GetSecret(ctx, db, databaseName)
		if errors.Is(err, sql.ErrNoRows) {
			if value := strings.TrimSpace(c.legacyAppSecretValues[path]); value != "" {
				if err := database.SetAppSecretValue(ctx, db, databaseName, value); err != nil {
					return err
				}
				secret, err = database.GetSecret(ctx, db, databaseName)
			}
		} else if err != nil {
			return fmt.Errorf("read app secret %s: %w", name, err)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read app secret %s: %w", name, err)
		}
		if secret != nil && secret.Value != "" && databaseName == name {
			c.setLegacyAppSecret(name, secret.Value)
		}
		if name == "installation_secret" {
			secret, err = database.EnsureSecret(ctx, db, name)
			if err != nil {
				return fmt.Errorf("ensure app secret %s: %w", name, err)
			}
			c.setLegacyAppSecret(name, secret.Value)
		}
	}
	if len(c.legacyAppSecretValues) == 0 || c.configPath == "" {
		return nil
	}
	if _, err := os.Stat(migrationBackupPath(c.configPath)); errors.Is(err, os.ErrNotExist) {
		if backupErr := backupConfig(c.configPath); backupErr != nil {
			slog.Warn("legacy application secrets imported but config cleanup skipped; backup could not be written", "path", c.configPath, "err", backupErr)
			return nil
		}
	} else if err != nil {
		return fmt.Errorf("inspect config migration backup: %w", err)
	}
	paths := []string{c.configPath}
	dropIns, err := configDropInPaths(c.configPath)
	if err != nil {
		return fmt.Errorf("list config drop-ins for legacy secret cleanup: %w", err)
	}
	paths = append(paths, dropIns...)
	paths = append(paths, filepath.Join(filepath.Dir(c.configPath), "secrets.yml"))
	for _, sourcePath := range paths {
		document, err := yamlNodeFromConfig(sourcePath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read %s during legacy secret cleanup: %w", sourcePath, err)
		}
		changed := false
		for _, name := range secretNames {
			changed = removeConfigPath(document, "server.security."+name) || changed
		}
		if changed {
			if err := writeYAMLAtomic(sourcePath, document, ".legacy-secret-*"); err != nil {
				return fmt.Errorf("remove imported legacy secrets from %s: %w", sourcePath, err)
			}
		}
	}
	c.legacyAppSecretValues = nil
	return nil
}

func (c *Config) setLegacyAppSecret(name, value string) {
	switch name {
	case "installation_secret":
		c.Server.Security.InstallationSecret = value
	case "installation_secret_previous":
		c.Server.Security.InstallationSecretPrevious = value
	case "installation_secret_previous_expires":
		c.Server.Security.InstallationSecretPreviousExpires = value
	case "encryption_key_previous":
		c.Server.Security.EncryptionKeyPrevious = value
	case "encryption_key_previous_expires":
		c.Server.Security.EncryptionKeyPreviousExpires = value
	}
}
