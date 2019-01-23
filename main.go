package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/brian1917/illumioapi"
	"stash.ilabs.io/ct/goabsorption/tablewriter"
)

type match struct {
	csname     string
	ipAddress  string
	hostname   string
	app        string
	env        string
	loc        string
	role       string
	reason     string
	wlhostname string
	eApp       string
	eEnv       string
	eLoc       string
	eRole      string
	wlHref     string
}

// Contains checks if an integer is in a slice
func containsInt(intSlice []int, searchInt int) bool {
	for _, value := range intSlice {
		if value == searchInt {
			return true
		}
	}
	return false
}

// ContainsStr hecks if an integer is in a slice
func containsStr(strSlice []string, searchStr string) bool {
	for _, value := range strSlice {
		if value == searchStr {
			return true
		}
	}
	return false
}

func main() {

	fqdn := flag.String("fqdn", "", "The fully qualified domain name of the PCE.")
	port := flag.Int("port", 8443, "The port for the PCE.")
	org := flag.Int("org", 1, "The org value for the PCE.")
	user := flag.String("user", "", "API user or email address.")
	pwd := flag.String("pwd", "", "API key if using API user or password if using email address.")
	app := flag.String("app", "", "App name. Explorer results focus on that app as provider or consumer. Default is all apps")
	csvFile := flag.String("in", "unmanaged-maker_default.csv", "CSV input file to be used to identify unmanaged workloads.")
	outputFile := flag.String("out", "umwl_output.csv", "File to write the unmanaged workloads to.")
	lookupTO := flag.Int("time", 1000, "Timeout to lookup hostname in ms.")
	consExcl := flag.String("excl", "", "Label to exclude as a consumer.")
	disableTLS := flag.Bool("x", false, "Disable TLS checking.")
	term := flag.Bool("t", false, "PrettyPrint the CSV to the terminal.")
	verbose := flag.Bool("v", false, "Verbose output provides additional columns in output to explain the match reason.")
	incWLs := flag.Bool("w", false, "Include IP addresses already assigned to workloads to suggest or verify labels.")
	privOnly := flag.Bool("p", false, "Limit suggested workloads to the RFC 1918 address space.")
	gat := flag.Bool("g", false, "Output CSV for GAT import. -w and -v are ignored with -g.")
	ilo := flag.Bool("ilo", false, "Output two CSVs (workloads and labels) to import via ILO-CLI. -w and -v are ignored with -i.")
	dupes := flag.Bool("d", false, "Allow IP address to have more than 1 recommendation. Default uses CSV order.")

	// Go's alphabetical ordering is annoying so writing out our own help menu (will eventually use a cli package)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		fmt.Println("-fqdn  string")
		fmt.Println("       The fully qualified domain name of the PCE. Required.")
		fmt.Println("-port  int")
		fmt.Println("       The port of the PCE. (default 8443)")
		fmt.Println("-org   int")
		fmt.Println("       The org value for the PCE. (default 1)")
		fmt.Println("-user  string")
		fmt.Println("       API user or email address. Required.")
		fmt.Println("-pwd   string")
		fmt.Println("       API key if using API user or password if using email address. Required.")
		fmt.Println("-app   string")
		fmt.Println("       App name. Explorer results focus on that app as provider or consumer. Default is all apps.")
		fmt.Println("-in    string")
		fmt.Println("       CSV input file to be used to identify unmanaged workloads. (default \"unmanaged-maker_default.csv\")")
		fmt.Println("-out   string")
		fmt.Println("       File to write the unmanaged workloads to. (default \"umwl_output.csv\")")
		fmt.Println("-time  int")
		fmt.Println("       Timeout to lookup hostname in ms. (default 1000)")
		fmt.Println("-excl  string")
		fmt.Println("       Label to exclude as a consumer role")
		fmt.Println("-x     Disable TLS checking.")
		fmt.Println("-t     PrettyPrint the CSV to the terminal.")
		// fmt.Println("-v     Verbose output provides additional columns in output to explain the match reason.") MADE DEFAULT YES CANNOT TURN OFF
		fmt.Println("-w     Include IP addresses already assigned to workloads to suggest or verify labels.")
		fmt.Println("-p     Limit suggested workloads to the RFC 1918 address space.")
		fmt.Println("-g     Output CSV for GAT import to create UMWLs. -w and -v are ignored with -g.")
		fmt.Println("-i     Output two CSVs (workloads and labels) to import via ILO-CLI. -w and -v are ignored with -i.")
		fmt.Println("-l     Appends columns to end of CSV for GAT labeling.")
	}

	// Parse flags
	flag.Parse()

	// If the GAT flag is set, we want to over-ride a few user-supplied flags
	if *gat {
		*incWLs = false
		*dupes = false
		*ilo = false
		*term = false
	}

	// If the ILO flag is set, we want to over-ride a few user-supplied flags
	if *ilo {
		*incWLs = false
		*dupes = false
		*term = false
	}

	// Set verbose output as true for default. Decide later if we want to remove the option
	*verbose = true

	// Run some quick checks on the required fields
	if len(*fqdn) == 0 || len(*user) == 0 || len(*pwd) == 0 {
		flag.Usage()
		log.Fatalf("ERROR - Required flags not included")
	}

	// If user is provided, we need to authenticate to the PCE
	userStr := *user
	if userStr[:4] != "api_" {
		auth, _, err := illumioapi.Authenticate(illumioapi.PCE{FQDN: *fqdn, Port: *port, Org: *org, DisableTLSChecking: *disableTLS}, *user, *pwd)
		if err != nil {
			log.Fatalf("Error - Authenticating to PCE - %s", err)
		}
		login, _, err := illumioapi.Login(illumioapi.PCE{FQDN: *fqdn, Port: *port, Org: *org, DisableTLSChecking: *disableTLS}, auth.AuthToken)
		if err != nil {
			log.Fatalf("Error - Logging in to PCE - %s", err)
		}
		user = &login.AuthUsername
		pwd = &login.SessionToken
	}

	// Create the PCE
	pce := illumioapi.PCE{
		FQDN:               *fqdn,
		Port:               *port,
		Org:                *org,
		User:               *user,
		Key:                *pwd,
		DisableTLSChecking: *disableTLS}

	// Parse the CSV
	coreServices := csvParser(*csvFile)

	// Get the label if we are going to do a consumer exclude
	var exclLabel illumioapi.Label
	if len(*consExcl) > 0 {
		exclLabel, _, err := illumioapi.GetLabel(pce, "role", *consExcl)
		if err != nil {
			log.Fatalf("ERROR - Getting label HREF - %s", err)
		}
		if exclLabel.Href == "" {
			log.Fatalf("ERROR- %s does not exist as an role label.", *consExcl)
		}
	}

	// Create the default query struct
	tq := illumioapi.TrafficQuery{
		StartTime:      time.Date(2013, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:        time.Date(2020, 12, 30, 0, 0, 0, 0, time.UTC),
		PolicyStatuses: []string{"allowed", "potentially_blocked", "blocked"},
		SourcesExclude: []string{exclLabel.Href},
		MaxFLows:       100000}

	// If an app is provided, we want to run with that app as the consumer.
	if *app != "" {
		label, _, err := illumioapi.GetLabel(pce, "app", *app)
		if err != nil {
			log.Fatalf("ERROR - Getting label HREF - %s", err)
		}
		if label.Href == "" {
			log.Fatalf("ERROR- %s does not exist as an app label.", *app)
		}
		tq.SourcesInclude = []string{label.Href}
	}

	traffic, err := illumioapi.GetTrafficAnalysis(pce, tq)
	if err != nil {
		log.Fatalf("ERROR - Making explorer API call - %s", err)
	}

	// Switch to the destination include, clear the sources include, run query again, append to previous result
	if *app != "" {
		tq.DestinationsInclude = tq.SourcesInclude
		tq.SourcesInclude = []string{}

		traffic2, err := illumioapi.GetTrafficAnalysis(pce, tq)
		if err != nil {
			log.Fatalf("ERROR - Making second explorer API call - %s", err)
		}
		traffic = append(traffic, traffic2...)
	}

	// Get Providers and Consumers and combine into one slice
	portProv := findPorts(traffic, coreServices, true, *incWLs)
	portCons := findPorts(traffic, coreServices, false, *incWLs)
	process := findProcesses(traffic, coreServices, *incWLs)

	matches := append(append(portProv, portCons...), process...)

	// Sort the Matches
	sMatches := []match{}
	for _, cs := range coreServices {
		for _, m := range matches {
			if m.csname == cs.name {
				sMatches = append(sMatches, m)
			}
		}
	}

	// Remove entries where the IP Address is assigned to a workload
	allIPNames := make(map[string]string)
	allIPWLs := make(map[string]illumioapi.Workload)

	wls, _, err := illumioapi.GetAllWorkloads(pce)
	if err != nil {
		log.Fatalf("ERROR - getting all workloads - %s", err)
	}
	for _, wl := range wls {

		for _, iface := range wl.Interfaces {
			name := wl.Name
			if net.ParseIP(wl.Hostname) == nil && len(wl.Hostname) > 0 {
				name = wl.Hostname
			}
			allIPNames[iface.Address] = name
			allIPWLs[iface.Address] = wl
		}
	}
	sNonWlMatches := []match{}
	sMatchesWLName := []match{}
	for _, sm := range sMatches {
		sm.wlhostname = allIPNames[sm.ipAddress]
		sm.wlHref = allIPWLs[sm.ipAddress].Href
		if _, ok := allIPNames[sm.ipAddress]; !ok {
			sm.wlhostname = "IP ONLY - NO WORKLOAD"
			sNonWlMatches = append(sNonWlMatches, sm)
		}
		sMatchesWLName = append(sMatchesWLName, sm)
	}

	// Assign the final matches
	fmBeforePriv := sNonWlMatches
	if *incWLs {
		fmBeforePriv = sMatchesWLName
	}

	// Remove nonRFC 1918 if flagged
	var finalMatches []match
	if *privOnly {

		for _, m := range fmBeforePriv {
			rfc1918 := []string{"192.168.0.0/16", "172.16.0.0/12", "10.0.0.0/8"}
			privCheck := false
			// Iterate through the three RFC 1918 ranges
			for _, cidr := range rfc1918 {
				// Get the ipv4Net
				_, ipv4Net, _ := net.ParseCIDR(cidr)
				// Check if it is in the range
				privCheck = ipv4Net.Contains(net.ParseIP(m.ipAddress))
				// If we get a true, append to the slice and stop checking the other ranges
				if privCheck {
					finalMatches = append(finalMatches, m)
					break
				}
			}
		}
	} else {
		// If the private only flag wasn't included, we use the previous slice
		finalMatches = fmBeforePriv
	}

	// Get the hostnames for the final matches
	var finalMatchesHost []match
	if *lookupTO > 0 {
		ctx, cancel := context.WithTimeout(context.TODO(), time.Duration(*lookupTO)*time.Millisecond)
		defer cancel()
		var r net.Resolver
		for _, fm := range finalMatches {
			if fm.wlhostname == "IP ONLY - NO WORKLOAD" {
				names, _ := r.LookupAddr(ctx, fm.ipAddress)
				if len(names) > 2 {
					fm.hostname = fmt.Sprintf("%s; %s; and %d more", names[0], names[1], len(names)-2)
				} else {
					fm.hostname = strings.Join(names, ";")
				}
				if fm.hostname == "" {
					fm.hostname = fmt.Sprintf("%s - %s", fm.ipAddress, fm.csname)
				}
			} else {
				fm.hostname = fm.wlhostname
			}
			finalMatchesHost = append(finalMatchesHost, fm)
		}
	} else {
		for _, fm := range finalMatches {
			if fm.wlhostname == "IP ONLY - NO WORKLOAD" {
				if fm.hostname == "" {
					fm.hostname = fmt.Sprintf("%s - %s", fm.ipAddress, fm.csname)
				}
			} else {
				fm.hostname = fm.wlhostname
			}
			finalMatchesHost = append(finalMatchesHost, fm)
		}
	}

	// PUT IN EXISTING LABELS - NEED TO CLEAN THIS UP LATER
	var finalMatchesHostWithExistingLabels []match
	allLabels := make(map[string]illumioapi.Label)
	labels, _, err := illumioapi.GetAllLabels(pce)
	if err != nil {
		log.Fatalf("ERROR - getting all workloads - %s", err)
	}

	// Populate Map
	for _, l := range labels {
		allLabels[l.Href] = l
	}

	for _, m := range finalMatchesHost {
		for _, l := range allIPWLs[m.ipAddress].Labels {
			switch {
			case allLabels[l.Href].Key == "app":
				{
					m.eApp = allLabels[l.Href].Value
				}
			case allLabels[l.Href].Key == "role":
				{
					m.eRole = allLabels[l.Href].Value
				}
			case allLabels[l.Href].Key == "env":
				{
					m.eEnv = allLabels[l.Href].Value
				}
			case allLabels[l.Href].Key == "loc":
				{
					m.eLoc = allLabels[l.Href].Value
				}
			}

		}
		finalMatchesHostWithExistingLabels = append(finalMatchesHostWithExistingLabels, m)
	}

	ipAddr := make(map[string]int)
	// Write out the CSV file
	fileName := *outputFile
	if *ilo {
		fileName = fmt.Sprintf("%s%s", "bulk_upload_csv-", *outputFile)
	}
	if len(matches) > 0 {
		file, err := os.Create(fileName)
		if err != nil {
			log.Fatalf("ERROR - Creating file - %s\n", err)
		}
		defer file.Close()

		// Write the headers
		switch {
		case *gat:
			{
				// GAT does not use headers - do nothing
			}
		case *ilo:
			{
				fmt.Fprintf(file, "hostname,ips,os_type\r\n")
			}
		case *verbose:
			{
				fmt.Fprintf(file, "ip_address,hostname,existing_workload,current_role,current_app,current_env,current_loc,match_reason,hostname,suggested_role,suggested_app,suggested_env,suggested_loc,href\r\n")
			}
		default:
			{
				fmt.Fprintf(file, "ip_address,hostname,app,role,env,loc\r\n")
			}
		}

		// Write the data
		for _, fmh := range finalMatchesHostWithExistingLabels {
			if _, ok := ipAddr[fmh.ipAddress]; !ok || *dupes {
				ipAddr[fmh.ipAddress] = 1
				wlCheck := "Yes"
				if fmh.wlhostname == "IP ONLY - NO WORKLOAD" {
					wlCheck = "No"
				}
				switch {
				case *gat:
					{
						fmt.Fprintf(file, "%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,eth0:%s,%s,%s\r\n", fmh.hostname, "", fmh.role, fmh.app, fmh.env, fmh.loc, fmh.hostname, "", "", "", fmh.ipAddress, "eth0:"+fmh.ipAddress, "", "")
					}
				case *ilo:
					{
						fmt.Fprintf(file, "%s,%s,%s\r\n", fmh.hostname, fmh.ipAddress, "")
					}
				case *verbose:
					{
						fmt.Fprintf(file, "%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\r\n", fmh.ipAddress, fmh.hostname, wlCheck, fmh.eRole, fmh.eApp, fmh.eEnv, fmh.eLoc, fmh.reason, fmh.hostname, fmh.role, fmh.app, fmh.env, fmh.loc, fmh.wlHref)
					}
				default:
					{
						fmt.Fprintf(file, "%s,%s,%s,%s,%s,%s\r\n", fmh.ipAddress, fmh.hostname, fmh.app, fmh.role, fmh.env, fmh.loc)
					}
				}

			}

		}

		// If ILO, we need to create a second CSV for the labels
		if *ilo {
			ipAddr := make(map[string]int)
			file, err := os.Create("label_sync_csv-" + *outputFile)
			if err != nil {
				log.Fatalf("ERROR - Creating file - %s\n", err)
			}
			defer file.Close()
			fmt.Fprintf(file, "role,app,env,loc,ips\r\n")
			for _, fmh := range finalMatchesHost {
				if _, ok := ipAddr[fmh.ipAddress]; !ok {
					ipAddr[fmh.ipAddress] = 1
					fmt.Fprintf(file, "%s,%s,%s,%s,%s\r\n", fmh.role, fmh.app, fmh.env, fmh.loc, fmh.ipAddress)
				}
			}

		}

		// Print to terminal if flagged
		if *term {
			table, err := tablewriter.NewCSV(os.Stdout, *outputFile, !*gat)
			if err != nil {
				log.Printf("Error - printing csv to terminal - %s", err)
			}
			table.SetAlignment(tablewriter.ALIGN_LEFT)
			table.SetRowLine(true)
			table.SetRowSeparator("-")
			table.Render()
		}
	} else {
		fmt.Println("NO MATCHES")
	}

}
