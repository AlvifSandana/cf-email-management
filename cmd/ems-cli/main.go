package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

type Config struct {
	ServerURL string
	APIKey    string
	JSONOut   bool
}

func main() {
	var cfg Config
	flag.StringVar(&cfg.ServerURL, "server", getEnv("EMS_SERVER_URL", "http://localhost:8080"), "EMS server base URL")
	flag.StringVar(&cfg.APIKey, "api-key", getEnv("EMS_API_KEY", "ems-admin-secret-key"), "EMS Admin API Key")
	flag.BoolVar(&cfg.JSONOut, "json", false, "Output results in JSON format")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Email Management Service CLI (ems-cli)\n\n")
		fmt.Fprintf(os.Stderr, "Usage: ems-cli [options] <command> <subcommand> [arguments...]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nCommands:\n")
		fmt.Fprintf(os.Stderr, "  health                              Check EMS server health\n")
		fmt.Fprintf(os.Stderr, "  accounts list                       List configured Cloudflare accounts\n")
		fmt.Fprintf(os.Stderr, "  accounts add <name> <accID> <token> Register a new Cloudflare account\n")
		fmt.Fprintf(os.Stderr, "  accounts delete <id>                Delete an account\n")
		fmt.Fprintf(os.Stderr, "  zones list                          List imported zones\n")
		fmt.Fprintf(os.Stderr, "  zones remote                        Discover remote zones from Cloudflare\n")
		fmt.Fprintf(os.Stderr, "  zones import <accID> <provZoneID>   Import a zone\n")
		fmt.Fprintf(os.Stderr, "  zones sync <zoneID> [direction]     Sync or drift-check zone (drift_check|pull|push)\n")
		fmt.Fprintf(os.Stderr, "  destinations list [accID]           List destination addresses\n")
		fmt.Fprintf(os.Stderr, "  destinations add <accID> <email>    Register destination address\n")
		fmt.Fprintf(os.Stderr, "  destinations delete <destID>        Delete destination address\n")
		fmt.Fprintf(os.Stderr, "  rules list <zoneID>                 List explicit rules for a zone\n")
		fmt.Fprintf(os.Stderr, "  rules add <zoneID> <name> <to> <action> <dest>  Add rule (action: forward|drop|worker)\n")
		fmt.Fprintf(os.Stderr, "  rules delete <zoneID> <ruleID>      Delete a rule\n")
		fmt.Fprintf(os.Stderr, "  catch-all get <zoneID>              Get catch-all configuration\n")
		fmt.Fprintf(os.Stderr, "  catch-all set <zoneID> <action> <dest> [enabled] Set catch-all rule\n")
		fmt.Fprintf(os.Stderr, "  audit list [limit]                  Show recent audit events\n")
	}

	flag.Parse()
	args := flag.Args()

	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	cmd := args[0]
	client := &http.Client{Timeout: 30 * time.Second}

	switch cmd {
	case "health":
		handleHealth(client, cfg)
	case "accounts":
		handleAccounts(client, cfg, args[1:])
	case "zones":
		handleZones(client, cfg, args[1:])
	case "destinations":
		handleDestinations(client, cfg, args[1:])
	case "rules":
		handleRules(client, cfg, args[1:])
	case "catch-all":
		handleCatchAll(client, cfg, args[1:])
	case "audit":
		handleAudit(client, cfg, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		flag.Usage()
		os.Exit(1)
	}
}

func getEnv(key, defVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defVal
}

func doReq(client *http.Client, cfg Config, method, path string, body any) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(data)
	}

	url := strings.TrimRight(cfg.ServerURL, "/") + path
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-API-Key", cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(respBytes))
	}

	return respBytes, nil
}

func printOutput(cfg Config, data any, prettyFunc func()) {
	if cfg.JSONOut {
		b, _ := json.MarshalIndent(data, "", "  ")
		fmt.Println(string(b))
		return
	}
	prettyFunc()
}

// --- Command Handlers ---

func handleHealth(client *http.Client, cfg Config) {
	respBytes, err := doReq(client, cfg, "GET", "/healthz", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Health check failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[OK] Server healthy: %s\n", string(respBytes))
}

func handleAccounts(client *http.Client, cfg Config, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Missing accounts subcommand: list, add, delete")
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		resp, err := doReq(client, cfg, "GET", "/api/v1/accounts", nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		var env struct {
			Data []map[string]any `json:"data"`
		}
		_ = json.Unmarshal(resp, &env)
		printOutput(cfg, env.Data, func() {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tCLOUDFLARE ACCOUNT ID\tSTATUS")
			for _, a := range env.Data {
				fmt.Fprintf(w, "%v\t%v\t%v\t%v\n", a["id"], a["name"], a["cloudflare_account_id"], a["status"])
			}
			w.Flush()
		})

	case "add":
		if len(args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: ems-cli accounts add <name> <cf-account-id> <api-token>")
			os.Exit(1)
		}
		payload := map[string]string{
			"name":                  args[1],
			"cloudflare_account_id": args[2],
			"api_token":             args[3],
		}
		resp, err := doReq(client, cfg, "POST", "/api/v1/accounts", payload)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("[+] Account created successfully:")
		var env map[string]any
		_ = json.Unmarshal(resp, &env)
		printOutput(cfg, env, func() {
			fmt.Println(string(resp))
		})

	case "delete":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: ems-cli accounts delete <id>")
			os.Exit(1)
		}
		_, err := doReq(client, cfg, "DELETE", "/api/v1/accounts/"+args[1], nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("[+] Account deleted successfully.")
	}
}

func handleZones(client *http.Client, cfg Config, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Missing zones subcommand: list, remote, import, sync")
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		resp, err := doReq(client, cfg, "GET", "/api/v1/zones", nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		var env struct {
			Data []map[string]any `json:"data"`
		}
		_ = json.Unmarshal(resp, &env)
		printOutput(cfg, env.Data, func() {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tPROVIDER ZONE ID\tROUTING\tSTATUS")
			for _, z := range env.Data {
				fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\n", z["id"], z["name"], z["provider_zone_id"], z["email_routing_enabled"], z["status"])
			}
			w.Flush()
		})

	case "remote":
		resp, err := doReq(client, cfg, "GET", "/api/v1/zones/remote", nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		var env struct {
			Data []map[string]any `json:"data"`
		}
		_ = json.Unmarshal(resp, &env)
		printOutput(cfg, env.Data, func() {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "REMOTE ZONE ID\tNAME\tSTATUS")
			for _, z := range env.Data {
				fmt.Fprintf(w, "%v\t%v\t%v\n", z["provider_zone_id"], z["name"], z["status"])
			}
			w.Flush()
		})

	case "import":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: ems-cli zones import <accountID> <providerZoneID>")
			os.Exit(1)
		}
		payload := map[string]string{
			"account_id":       args[1],
			"provider_zone_id": args[2],
		}
		resp, err := doReq(client, cfg, "POST", "/api/v1/zones/import", payload)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[+] Zone imported successfully: %s\n", string(resp))

	case "sync":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: ems-cli zones sync <zoneID> [drift_check|pull|push]")
			os.Exit(1)
		}
		dir := "drift_check"
		if len(args) >= 3 {
			dir = args[2]
		}
		payload := map[string]string{"direction": dir}
		resp, err := doReq(client, cfg, "POST", "/api/v1/zones/"+args[1]+"/sync", payload)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		var env struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(resp, &env)
		printOutput(cfg, env.Data, func() {
			d := env.Data
			fmt.Printf("Sync Run: %v | Status: %v | Changes: %v | Errors: %v\n", d["run_id"], d["status"], d["changes_count"], d["error_count"])
			if diffs, ok := d["diffs"].([]any); ok && len(diffs) > 0 {
				fmt.Println("\nDiff Items:")
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "STATUS\tRESOURCE\tIDENTIFIER\tDETAILS")
				for _, df := range diffs {
					m := df.(map[string]any)
					fmt.Fprintf(w, "%v\t%v\t%v\t%v\n", m["status"], m["resource_type"], m["identifier"], m["details"])
				}
				w.Flush()
			}
		})
	}
}

func handleDestinations(client *http.Client, cfg Config, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Missing destinations subcommand: list, add, delete")
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		path := "/api/v1/destinations"
		if len(args) >= 2 {
			path += "?account_id=" + args[1]
		}
		resp, err := doReq(client, cfg, "GET", path, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		var env struct {
			Data []map[string]any `json:"data"`
		}
		_ = json.Unmarshal(resp, &env)
		printOutput(cfg, env.Data, func() {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tEMAIL\tSTATUS\tACCOUNT ID")
			for _, d := range env.Data {
				fmt.Fprintf(w, "%v\t%v\t%v\t%v\n", d["id"], d["email"], d["status"], d["provider_account_id"])
			}
			w.Flush()
		})

	case "add":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: ems-cli destinations add <accountID> <email>")
			os.Exit(1)
		}
		payload := map[string]string{
			"account_id": args[1],
			"email":      args[2],
		}
		resp, err := doReq(client, cfg, "POST", "/api/v1/destinations", payload)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[+] Destination created: %s\n", string(resp))

	case "delete":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: ems-cli destinations delete <destID>")
			os.Exit(1)
		}
		_, err := doReq(client, cfg, "DELETE", "/api/v1/destinations/"+args[1], nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("[+] Destination deleted.")
	}
}

func handleRules(client *http.Client, cfg Config, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Missing rules subcommand: list, add, delete")
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: ems-cli rules list <zoneID>")
			os.Exit(1)
		}
		resp, err := doReq(client, cfg, "GET", "/api/v1/zones/"+args[1]+"/rules", nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		var env struct {
			Data []map[string]any `json:"data"`
		}
		_ = json.Unmarshal(resp, &env)
		printOutput(cfg, env.Data, func() {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tMATCHER\tACTION\tDESTINATION\tENABLED")
			for _, r := range env.Data {
				fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\t%v\n", r["id"], r["name"], r["matcher_value"], r["action_type"], r["destination"], r["enabled"])
			}
			w.Flush()
		})

	case "add":
		if len(args) < 6 {
			fmt.Fprintln(os.Stderr, "Usage: ems-cli rules add <zoneID> <name> <to-address> <action(forward|drop|worker)> <destination>")
			os.Exit(1)
		}
		payload := map[string]any{
			"name":          args[2],
			"matcher_type":  "literal",
			"matcher_field": "to",
			"matcher_value": args[3],
			"action_type":   args[4],
			"destination":   args[5],
			"enabled":       true,
		}
		resp, err := doReq(client, cfg, "POST", "/api/v1/zones/"+args[1]+"/rules", payload)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[+] Rule created: %s\n", string(resp))

	case "delete":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: ems-cli rules delete <zoneID> <ruleID>")
			os.Exit(1)
		}
		_, err := doReq(client, cfg, "DELETE", "/api/v1/zones/"+args[1]+"/rules/"+args[2], nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("[+] Rule deleted.")
	}
}

func handleCatchAll(client *http.Client, cfg Config, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Missing catch-all subcommand: get, set")
		os.Exit(1)
	}

	switch args[0] {
	case "get":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: ems-cli catch-all get <zoneID>")
			os.Exit(1)
		}
		resp, err := doReq(client, cfg, "GET", "/api/v1/zones/"+args[1]+"/catch-all", nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		var env struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(resp, &env)
		printOutput(cfg, env.Data, func() {
			d := env.Data
			fmt.Printf("Catch-All Status: Enabled=%v | Action=%v | Destination=%v\n", d["enabled"], d["action_type"], d["destination"])
		})

	case "set":
		if len(args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: ems-cli catch-all set <zoneID> <action(forward|drop|worker)> <destination> [enabled(true|false)]")
			os.Exit(1)
		}
		enabled := true
		if len(args) >= 5 && strings.ToLower(args[4]) == "false" {
			enabled = false
		}
		payload := map[string]any{
			"action_type": args[2],
			"destination": args[3],
			"enabled":     enabled,
		}
		resp, err := doReq(client, cfg, "PUT", "/api/v1/zones/"+args[1]+"/catch-all", payload)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[+] Catch-All updated: %s\n", string(resp))
	}
}

func handleAudit(client *http.Client, cfg Config, args []string) {
	limit := "20"
	if len(args) >= 2 {
		limit = args[1]
	}
	resp, err := doReq(client, cfg, "GET", "/api/v1/audit-events?limit="+limit, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	var env struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.Unmarshal(resp, &env)
	printOutput(cfg, env.Data, func() {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TIMESTAMP\tOPERATION\tRESOURCE\tID\tSTATUS")
		for _, a := range env.Data {
			fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\n", a["created_at"], a["operation"], a["resource_type"], a["resource_id"], a["status"])
		}
		w.Flush()
	})
}
