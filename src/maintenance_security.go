package main

import (
	"context"
	"encoding/hex"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/apimgr/search/src/common/closeutil"
	"github.com/apimgr/search/src/common/display"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/database"
	"github.com/apimgr/search/src/email"
	"github.com/apimgr/search/src/logging"
	"github.com/apimgr/search/src/security"
)

var securityStatusOrder = map[string]int{"received": 0, "triaged": 1, "confirmed": 2, "patching": 3, "disclosed": 4, "wont_fix": 5}

func securityMaintenanceUsageError(action string, args []string) (string, string, bool) {
	usagePrefix := fmt.Sprintf("Usage: %s --maintenance security", filepath.Base(os.Args[0]))
	usageByAction := map[string]string{
		"list":   usagePrefix + " list [--status STATUS]",
		"show":   usagePrefix + " show TRACKING_ID",
		"export": usagePrefix + " export TRACKING_ID [--output FILE]",
		"update": usagePrefix + " update TRACKING_ID [--status STATUS] [--comment TEXT] [--disclosure-date YYYY-MM-DD] [--severity LEVEL]",
	}
	usage, known := usageByAction[action]
	if !known {
		if action == "" || action == "help" || action == "--help" {
			return "", "", false
		}
		return fmt.Sprintf("unknown security action %q", action), usagePrefix+" list|show|export|update ...", true
	}
	invalid := func(reason string) (string, string, bool) {
		return reason, usage, true
	}
	switch action {
	case "list":
		fs := flag.NewFlagSet("security list", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		status := fs.String("status", "", "filter by status")
		if err := fs.Parse(args); err != nil {
			return invalid(err.Error())
		}
		if fs.NArg() != 0 {
			return invalid(fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
		}
		if *status != "" {
			if _, ok := securityStatusOrder[*status]; !ok {
				return invalid(fmt.Sprintf("invalid status %q", *status))
			}
		}
	case "show":
		if len(args) != 1 {
			return invalid("show requires one tracking ID")
		}
		if !validSecurityTrackingID(args[0]) {
			return invalid("invalid tracking ID")
		}
	case "export":
		if len(args) == 0 {
			return invalid("export requires a tracking ID")
		}
		if !validSecurityTrackingID(args[0]) {
			return invalid("invalid tracking ID")
		}
		fs := flag.NewFlagSet("security export", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		fs.String("output", "", "output path or - for stdout")
		if err := fs.Parse(args[1:]); err != nil {
			return invalid(err.Error())
		}
		if fs.NArg() != 0 {
			return invalid(fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
		}
	case "update":
		if len(args) == 0 {
			return invalid("update requires a tracking ID")
		}
		if !validSecurityTrackingID(args[0]) {
			return invalid("invalid tracking ID")
		}
		fs := flag.NewFlagSet("security update", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		status := fs.String("status", "", "new status")
		comment := fs.String("comment", "", "researcher-visible comment")
		disclosure := fs.String("disclosure-date", "", "expected disclosure date")
		severity := fs.String("severity", "", "severity level")
		if err := fs.Parse(args[1:]); err != nil {
			return invalid(err.Error())
		}
		if fs.NArg() != 0 {
			return invalid(fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
		}
		if *status == "" && *comment == "" && *disclosure == "" && *severity == "" {
			return invalid("update requires at least one field")
		}
	}
	return "", "", false
}

func runMaintenanceSecurity(action string, args []string) {
	if reason, usage, invalid := securityMaintenanceUsageError(action, args); invalid {
		fmt.Fprintf(os.Stderr, "Error: %s\n", reason)
		fmt.Fprintln(os.Stderr, usage)
		exitFunc(2)
		return
	}
	if action == "help" || action == "--help" || action == "" {
		fmt.Println("Security report maintenance:")
		fmt.Println("  list [--status STATUS]")
		fmt.Println("  show TRACKING_ID")
		fmt.Println("  export TRACKING_ID [--output FILE]")
		fmt.Println("  update TRACKING_ID [--status STATUS] [--comment TEXT] [--disclosure-date YYYY-MM-DD] [--severity LEVEL]")
		return
	}
	cfg := pgpRequireAuthorized()
	if cfg == nil { return }
	dm, err := pgpOpenDB()
	if err != nil { fmt.Fprintf(os.Stderr, "Error: open security report database: %v\n", err); exitFunc(1); return }
	defer closeutil.Close(dm)
	if err := pgpMigrateLegacyReports(cfg, dm); err != nil {
		fmt.Fprintf(os.Stderr, "Error: migrate legacy security reports: %v\n", err)
		exitFunc(1)
		return
	}
	db := dm.ServerDB()
	table := database.ServerTableName(db, "security_reports")
	switch action {
	case "list":
		fs := flag.NewFlagSet("security list", flag.ContinueOnError); fs.SetOutput(io.Discard)
		statusFilter := fs.String("status", "", "filter by status")
		if err := fs.Parse(args); err != nil || fs.NArg() != 0 { fmt.Fprintln(os.Stderr, "Usage: --maintenance security list [--status STATUS]"); exitFunc(2); return }
		if *statusFilter != "" { if _, ok := securityStatusOrder[*statusFilter]; !ok { fmt.Fprintln(os.Stderr, "Error: invalid status"); exitFunc(2); return } }
		query := fmt.Sprintf("SELECT tracking_id,status,severity,component,created_at,updated_at FROM %s", table)
		var rows *database.TimedRows
		if *statusFilter == "" { rows, err = db.Query(context.Background(), query+" ORDER BY created_at DESC") } else { rows, err = db.Query(context.Background(), query+" WHERE status = ? ORDER BY created_at DESC", *statusFilter) }
		if err != nil { fmt.Fprintf(os.Stderr, "Error: list security reports: %v\n", err); exitFunc(1); return }
		fmt.Println("TRACKING_ID\tSTATUS\tSEVERITY\tCOMPONENT\tCREATED\tUPDATED")
		for rows.Next() {
			var id, status, severity, component, created, updated string
			if err := rows.Scan(&id, &status, &severity, &component, &created, &updated); err != nil {
				closeErr := rows.Close()
				fmt.Fprintf(os.Stderr, "Error: read security report: %v\n", errors.Join(err, closeErr))
				exitFunc(1)
				return
			}
			fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\n", id, status, severity, component, created, updated)
		}
		rowsErr := rows.Err()
		closeErr := rows.Close()
		if err := errors.Join(rowsErr, closeErr); err != nil {
			fmt.Fprintf(os.Stderr, "Error: iterate security reports: %v\n", err)
			exitFunc(1)
		}
	case "show":
		if len(args) != 1 || !validSecurityTrackingID(args[0]) { fmt.Fprintln(os.Stderr,"Usage: --maintenance security show TRACKING_ID"); exitFunc(2); return }
		var id,status,severity,component,comment string; var disclosure sql.NullString
		err = db.QueryRow(context.Background(),fmt.Sprintf("SELECT tracking_id,status,severity,component,researcher_comment,disclosure_date FROM %s WHERE tracking_id = ?",table),args[0]).Scan(&id,&status,&severity,&component,&comment,&disclosure)
		if err != nil { fmt.Fprintf(os.Stderr,"Error: read security report: %v\n",err); exitFunc(1); return }
		fmt.Printf("Tracking ID: %s\nStatus: %s\nSeverity: %s\nComponent: %s\nResearcher comment: %s\n",id,status,severity,component,comment)
		if disclosure.Valid { fmt.Printf("Disclosure date: %s\n",disclosure.String) }
	case "export":
		fs := flag.NewFlagSet("security export", flag.ContinueOnError); fs.SetOutput(io.Discard)
		output := fs.String("output", "", "output path or - for stdout")
		if len(args)==0 { fmt.Fprintln(os.Stderr,"Usage: --maintenance security export TRACKING_ID [--output FILE]"); exitFunc(2); return }
		id := args[0]
		if !validSecurityTrackingID(id) { fmt.Fprintln(os.Stderr,"Error: invalid tracking ID"); exitFunc(2); return }
		if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 { fmt.Fprintln(os.Stderr,"Usage: --maintenance security export TRACKING_ID [--output FILE]"); exitFunc(2); return }
		var body []byte
		if err := db.QueryRow(context.Background(),fmt.Sprintf("SELECT body_enc FROM %s WHERE tracking_id = ?",table),id).Scan(&body); err != nil { fmt.Fprintf(os.Stderr,"Error: read encrypted security report: %v\n",err); exitFunc(1); return }
		if *output == "-" { _, err = os.Stdout.Write(body) } else { path := *output; if path == "" { path = id+".asc" }; file, openErr := os.OpenFile(path,os.O_CREATE|os.O_TRUNC|os.O_WRONLY,0o600); if openErr != nil { err=openErr } else { _,err=file.Write(body); err=errors.Join(err,file.Close()); if err==nil { err=os.Chmod(path,0o600) } } }
		if err != nil { fmt.Fprintf(os.Stderr,"Error: write encrypted report: %v\n",err); exitFunc(1); return }
		logging.NewManager(config.GetLogDir()).Security().Log(logging.SecurityEntry{Timestamp:time.Now().UTC(),Event:logging.SecurityEventReportExported,IP:"-",Details:"tracking_id="+id})
	case "update":
		fs := flag.NewFlagSet("security update",flag.ContinueOnError); fs.SetOutput(io.Discard)
		status := fs.String("status","","new status"); comment := fs.String("comment","","researcher-visible comment"); disclosure := fs.String("disclosure-date","","expected disclosure date"); severity := fs.String("severity","","severity level")
		if len(args)==0 { fmt.Fprintln(os.Stderr,"Usage: --maintenance security update TRACKING_ID [--status STATUS] [--comment TEXT] [--disclosure-date YYYY-MM-DD] [--severity LEVEL]"); exitFunc(2); return }
		id:=args[0]
		if !validSecurityTrackingID(id) { fmt.Fprintln(os.Stderr,"Error: invalid tracking ID"); exitFunc(2); return }
		if err:=fs.Parse(args[1:]); err!=nil || fs.NArg()!=0 || (*status==""&&*comment==""&&*disclosure==""&&*severity=="") { fmt.Fprintln(os.Stderr,"Usage: --maintenance security update TRACKING_ID [--status STATUS] [--comment TEXT] [--disclosure-date YYYY-MM-DD] [--severity LEVEL]"); exitFunc(2); return }
		oldStatus:=""; var creditPref,creditName,reportSeverity string
		err=db.Write(context.Background(),func(tx *sql.Tx) error {
			if err:=tx.QueryRow(fmt.Sprintf("SELECT status,credit_pref,credit_name,severity FROM %s WHERE tracking_id = ?",table),id).Scan(&oldStatus,&creditPref,&creditName,&reportSeverity); err!=nil{return err}
			newStatus:=oldStatus
			if *status!="" { if _,ok:=securityStatusOrder[*status]; !ok{return fmt.Errorf("invalid status %q",*status)}; if *status!="wont_fix"&&securityStatusOrder[*status]<securityStatusOrder[oldStatus]{return fmt.Errorf("status cannot move backward")}; newStatus=*status }
			if *comment!=""&&len([]rune(*comment))>2000{return fmt.Errorf("comment exceeds 2000 characters")}
			if *disclosure!="" { if _,err:=time.Parse("2006-01-02",*disclosure);err!=nil{return fmt.Errorf("disclosure date must be YYYY-MM-DD") } }
			if *severity!="" { switch *severity {case "critical","high","medium","low","informational": default:return fmt.Errorf("invalid severity")}; reportSeverity=*severity }
			closedAt:=""; if newStatus=="disclosed"||newStatus=="wont_fix" { closedAt=time.Now().UTC().Format(time.RFC3339) }
			_,err:=tx.Exec(fmt.Sprintf("UPDATE %s SET status=?, researcher_comment=CASE WHEN ?='' THEN researcher_comment ELSE ? END, disclosure_date=CASE WHEN ?='' THEN disclosure_date ELSE ? END, severity=?, closed_at=CASE WHEN ?='' THEN closed_at ELSE ? END, updated_at=? WHERE tracking_id=?",table),newStatus,*comment,*comment,*disclosure,*disclosure,reportSeverity,closedAt,closedAt,time.Now().UTC().Format(time.RFC3339),id); if err!=nil{return err}
			if newStatus=="disclosed"&&oldStatus!="disclosed"&&creditPref!="no" { credit:=creditName; if creditPref=="anon" { var count int; if err:=tx.QueryRow("SELECT COUNT(*) FROM "+database.ServerTableName(db,"security_thanks")+" WHERE credit LIKE 'Anonymous Researcher #%'").Scan(&count);err!=nil{return err}; credit=fmt.Sprintf("Anonymous Researcher #%d",count+1) }; if credit==""{return fmt.Errorf("disclosed report has no credit name")}; thanks:=database.ServerTableName(db,"security_thanks"); _,err=tx.Exec(fmt.Sprintf("INSERT INTO %s (tracking_id,credit,severity,disclosed_on) VALUES (?,?,?,?) ON CONFLICT(tracking_id) DO NOTHING",thanks),id,credit,reportSeverity,time.Now().UTC().Format("2006-01-02")); if err!=nil{return err} }
			return nil
		})
		if err!=nil { fmt.Fprintf(os.Stderr,"Error: update security report: %v\n",err); exitFunc(1); return }
		newStatus:=oldStatus;if *status!=""{newStatus=*status}
		logging.NewManager(config.GetLogDir()).Security().Log(logging.SecurityEntry{Timestamp:time.Now().UTC(),Event:logging.SecurityEventReportUpdated,IP:"-",Details:fmt.Sprintf("tracking_id=%s old_status=%s new_status=%s",id,oldStatus,newStatus)})
		if cfg.Server.Notifications.Email.Enabled { sendSecurityStatusNotice(cfg,db,table,id,*status) }
		fmt.Println(display.Emoji("✅","[OK]")+" Security report updated")
	default:
		fmt.Fprintln(os.Stderr,"Usage: --maintenance security list|show|export|update ..."); exitFunc(2)
	}
}

func validSecurityTrackingID(id string) bool {
	if len(id) != len(security.TrackingIDPrefix)+security.TrackingIDRandomHexChars || id[:len(security.TrackingIDPrefix)] != security.TrackingIDPrefix { return false }
	_, err := hex.DecodeString(id[len(security.TrackingIDPrefix):])
	return err == nil
}

func sendSecurityStatusNotice(cfg *config.Config, db *database.DB, table, id, newStatus string) {
	if newStatus=="" { return }
	var encrypted []byte
	if err:=db.QueryRow(context.Background(),fmt.Sprintf("SELECT contact_enc FROM %s WHERE tracking_id = ?",table),id).Scan(&encrypted);err!=nil||len(encrypted)==0{return}
	plain,err:=security.DecryptAESGCM(cfg.Server.Security.EncryptionKey,string(encrypted));if err!=nil{return}
	var contact struct{Email string `json:"email"`};if json.Unmarshal(plain,&contact)!=nil||contact.Email==""{return}
	mailer,ok:=email.NewMailerFromConfig(cfg);if !ok{return}
	message:=email.NewMessage([]string{contact.Email},"Security report status updated",fmt.Sprintf("The status of security report %s is now %s.",id,newStatus))
	if err:=mailer.Send(message);err!=nil{fmt.Fprintf(os.Stderr,"Warning: status notice could not be queued: %v\n",err)}
}
