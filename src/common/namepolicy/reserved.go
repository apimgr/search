package namepolicy

import (
	"path"
	"strings"

	"golang.org/x/text/unicode/norm"
)

var reserved = map[string]struct{}{
	"api": {}, "server": {}, "static": {}, "assets": {}, "healthz": {}, "health": {}, "status": {}, "metrics": {}, "version": {},
	"webhook": {}, "webhooks": {}, "internal": {}, "private": {}, "public": {}, "well-known": {}, "cgi-bin": {}, "tmp": {}, "temp": {},
	"backup": {}, "backups": {}, "debug": {}, "test": {}, "dev": {}, "staging": {}, "null": {}, "undefined": {}, "nil": {}, "true": {}, "false": {}, "default": {}, "index": {}, "home": {}, "main": {},
	"admin": {}, "administrator": {}, "administration": {}, "root": {}, "superuser": {}, "sysadmin": {}, "sysop": {}, "system": {}, "sys": {}, "operator": {}, "owner": {}, "staff": {}, "moderator": {}, "mod": {}, "manager": {}, "management": {}, "manage": {}, "console": {}, "panel": {}, "cpanel": {}, "backend": {}, "config": {}, "configuration": {}, "wp-admin": {}, "wp-login": {}, "phpmyadmin": {}, "adminer": {}, "webmaster": {}, "postmaster": {}, "hostmaster": {}, "abuse": {}, "noreply": {}, "no-reply": {}, "bot": {}, "bots": {}, "anonymous": {}, "guest": {}, "everyone": {}, "nobody": {}, "me": {}, "self": {}, "www": {}, "mail": {}, "ftp": {}, "smtp": {}, "imap": {}, "pop": {}, "ns": {}, "ns1": {}, "ns2": {}, "localhost": {},
	"login": {}, "logout": {}, "register": {}, "signup": {}, "signin": {}, "signout": {}, "auth": {}, "oauth": {}, "callback": {}, "account": {}, "accounts": {}, "profile": {}, "settings": {}, "dashboard": {}, "user": {}, "users": {}, "org": {}, "orgs": {}, "organization": {}, "organizations": {}, "team": {}, "teams": {}, "group": {}, "groups": {}, "password": {}, "reset": {}, "verify": {}, "invite": {}, "invites": {}, "preferences": {},
	"search": {}, "explore": {}, "discover": {}, "trending": {}, "help": {}, "support": {}, "docs": {}, "documentation": {}, "about": {}, "contact": {}, "terms": {}, "privacy": {}, "legal": {}, "security": {}, "info": {}, "new": {}, "edit": {}, "delete": {}, "create": {}, "update": {}, "all": {}, "download": {}, "downloads": {}, "upload": {}, "billing": {}, "sales": {}, "hello": {},
	"graphql": {}, "mcp": {}, "swagger": {}, "rest": {}, "rpc": {}, "ws": {}, "websocket": {}, "cdn": {}, "media": {}, "uploads": {}, "files": {}, "images": {}, "css": {}, "js": {}, "fonts": {}, "img": {},
	".well-known": {}, "robots.txt": {}, "sitemap.xml": {}, "feed.xml": {}, "rss.xml": {}, "atom.xml": {}, "favicon.ico": {}, "manifest.json": {}, "humans.txt": {}, "security.txt": {}, "llms.txt": {}, "ads.txt": {}, "feed": {}, "rss": {}, "atom": {}, "sitemap": {}, "robots": {}, "favicon": {}, "manifest": {}, "browserconfig.xml": {}, "sw.js": {}, "service-worker.js": {},
}

var reservedExtensions = map[string]struct{}{
	".xml": {}, ".txt": {}, ".json": {}, ".ico": {}, ".html": {}, ".htm": {}, ".php": {}, ".js": {}, ".css": {}, ".map": {}, ".webmanifest": {}, ".png": {}, ".jpg": {}, ".svg": {}, ".zip": {},
}

var privileged = []string{
	"admin", "administrator", "administration", "root", "superuser", "sysadmin", "sysop", "system", "sys", "operator", "owner", "staff", "moderator", "mod", "manager", "management", "manage", "console", "panel", "cpanel", "backend", "config", "configuration", "wp-admin", "wp-login", "phpmyadmin", "adminer", "webmaster", "postmaster", "hostmaster", "abuse", "noreply", "no-reply", "bot", "bots", "anonymous", "guest", "everyone", "nobody", "me", "self", "www", "mail", "ftp", "smtp", "imap", "pop", "ns", "ns1", "ns2", "localhost",
}

func IsReserved(candidate string) bool {
	name := strings.ToLower(norm.NFKC.String(strings.ToLower(strings.TrimSpace(candidate))))
	name = strings.TrimPrefix(name, "@")
	if name == "" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
		return true
	}
	if _, ok := reserved[name]; ok {
		return true
	}
	if ext := strings.ToLower(path.Ext(name)); ext != "" {
		if _, ok := reservedExtensions[ext]; ok {
			return true
		}
	}
	for _, key := range privileged {
		if strings.HasPrefix(name, key) && len(name) > len(key) && allASCIIDigits(strings.TrimPrefix(name, key)) {
			return true
		}
	}
	return false
}

func allASCIIDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
