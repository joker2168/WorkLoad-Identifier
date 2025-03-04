package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type coreService struct {
	name               string
	provider           bool
	requiredPorts      []int
	optionalPorts      []int
	optionalPortRanges [][]int
	numOptionalPorts   int
	numFlows           int
	processes          []string
	numProcessesReq    int
	app                string
	env                string
	loc                string
	role               string
}

//struct to save subnet to location and environment labels
type subnetLabel struct {
	network  net.IPNet
	locLabel string
	envLabel string
}

// used to parse subnet to environment and location labels
func locParser(filename string) []subnetLabel {
	var netlabel []subnetLabel

	// column in the CSV
	networks := 0
	loclabel := 1
	envlabel := 2

	tmpFile, _ := os.Open(filename)
	reader := csv.NewReader(bufio.NewReader(tmpFile))

	i := 0
	for {

		i++
		tmp := subnetLabel{}
		line, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			log.Fatalf("Error - Reading CSV File - %s", err)
		}
		//ignore CSV header
		if i != 1 {

			//make sure location label not empty
			if line[loclabel] == "" {
				log.Fatal("Error - Label field cannot be empty")
			}

			//Place subnet into net.IPNet data structure as part of subnetLabel struct
			_, network, err := net.ParseCIDR(line[networks])
			if err != nil {
				log.Fatal("Error - The Subnet field cannot be parsed.  The format is 10.10.10.0/24")
			}

			//Set struct values
			tmp.network = *network
			tmp.envLabel = line[envlabel]
			tmp.locLabel = line[loclabel]
			netlabel = append(netlabel, tmp)
		}

	}
	return netlabel
}

func csvParser(filename string) []coreService {

	// Set CSV columns here to avoid changing multiple locations
	csvName := 0
	csvProvider := 1
	csvReqPorts := 2
	csvOptPorts := 3
	csvNumOptPorts := 4
	csvNumFlows := 5
	csvProcesses := 6
	csvNumProcess := 7
	csvRole := 8
	csvApp := 9
	csvEnv := 10
	csvLoc := 11

	var coreServices []coreService

	// Open CSV File
	csvFile, _ := os.Open(filename)
	reader := csv.NewReader(bufio.NewReader(csvFile))

	// Start the counters
	i := 0

	for {
		// Reset variables
		reqPortsInt := []int{}
		optPortsInt := []int{}
		optPortRangesInt := [][]int{}
		numOptPorts := 0
		numProcessesReq := 0
		numFlows := 0

		// Increment the counter
		i++

		// Read the CSV
		line, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			log.Fatalf("Error - Reading CSV File - %s", err)
		}

		// Skip the header row
		if i != 1 {

			// Set provider
			provider := true
			if line[csvProvider] == "0" {
				provider = false
			}
			// Set the required ports slice if there is any text in the field
			if len(line[csvReqPorts]) > 0 {
				requiredPortsStr := strings.Split(line[csvReqPorts], " ")
				for _, strPort := range requiredPortsStr {
					intPort, err := strconv.Atoi(strPort)
					if err != nil {
						log.Fatalf("ERROR - Converting required port to int on line %d - %s", i, err)
					}
					reqPortsInt = append(reqPortsInt, intPort)
				}
			}

			// Set the optional ports slice if there is any text in the field
			if len(line[csvOptPorts]) > 0 {

				// Split based on spaces
				optPortsStr := strings.Split(line[csvOptPorts], " ")

				for _, strPort := range optPortsStr {
					rangePortInt := []int{}

					// Process the entry if it a range
					rangePortStr := strings.Split(strPort, "-")
					if len(rangePortStr) > 1 {
						for _, rangeValue := range rangePortStr {
							value, err := strconv.Atoi(rangeValue)
							if err != nil {
								log.Fatalf("ERROR - Converting port range values to int on line %d - %s", i, err)
							}
							rangePortInt = append(rangePortInt, value)
						}
						optPortRangesInt = append(optPortRangesInt, rangePortInt)
					}

					// Process the entry if it is a single port
					if len(rangePortInt) == 0 {
						intPort, err := strconv.Atoi(strPort)
						if err != nil {
							log.Fatalf("ERROR - Converting optional port to int on line %d - %s", i, err)
						}
						optPortsInt = append(optPortsInt, intPort)
					}
				}
			}

			// Convert the number of optional ports to int if there is any text in the field
			if len(line[csvNumOptPorts]) > 0 {
				numOptPorts, err = strconv.Atoi(line[csvNumOptPorts])
				if err != nil {
					log.Fatalf("ERROR - Converting number of required ports to int on line %d - %s", i, err)
				}
			}

			// Convert the number of flows to int
			if len(line[csvNumFlows]) > 0 {
				numFlows, err = strconv.Atoi(line[csvNumFlows])
				if err != nil {
					log.Fatalf("ERROR - Converting number of flows to int on line %d - %s", i, err)
				}
			}

			// Convert the number of processes to int if there is any text in the field
			if len(line[6]) > 0 {
				numProcessesReq, err = strconv.Atoi(line[csvNumProcess])
				if err != nil {
					log.Fatalf("ERROR - Converting number of required consumer services to int on line %d - %s", i, err)
				}
			}

			// Append to the coreServices slice
			coreServices = append(coreServices, coreService{
				name:               line[csvName],
				provider:           provider,
				requiredPorts:      reqPortsInt,
				optionalPorts:      optPortsInt,
				optionalPortRanges: optPortRangesInt,
				numFlows:           numFlows,
				numOptionalPorts:   numOptPorts,
				processes:          strings.Split(line[csvProcesses], " "),
				numProcessesReq:    numProcessesReq,
				app:                line[csvApp],
				env:                line[csvEnv],
				loc:                line[csvLoc],
				role:               line[csvRole]})

		}
	}

	return coreServices

}

func csvWriter(matches []result, ilo, gat, exclWLs, nonMatchIncl bool) {

	// Get time stamp for output files
	timestamp := time.Now().Format("20060102_150405")

	var defaultFile, gatUmwlFile, gatLabelFile, iloUmwlFile, iloLabelFile *os.File

	// Always create the default file
	defaultFile, err := os.Create("identified-workloads_" + timestamp + ".csv")
	if err != nil {
		log.Fatalf("ERROR - Creating file - %s\n", err)
	}
	defer defaultFile.Close()
	fmt.Fprintf(defaultFile, "ip_address,name,status,current_role,current_app,current_env,current_loc,suggested_role,suggested_app,suggested_env,suggested_loc,reason\r\n")

	// Export GAT format if requested
	if gat {
		gatUmwlFile, err = os.Create("gat-create-umwls_" + timestamp + ".csv")
		if err != nil {
			log.Fatalf("ERROR - Creating file - %s\n", err)
		}
		defer gatUmwlFile.Close()
		gatLabelFile, err = os.Create("gat-update-labels_" + timestamp + ".csv")
		if err != nil {
			log.Fatalf("ERROR - Creating file - %s\n", err)
		}
		defer gatLabelFile.Close()
	}

	// Export ILO format if requested
	if ilo {
		iloUmwlFile, err = os.Create("ilo-create-umwls_" + timestamp + ".csv")
		if err != nil {
			log.Fatalf("ERROR - Creating file - %s\n", err)
		}
		defer iloUmwlFile.Close()
		iloLabelFile, err = os.Create("ilo-update-labels_" + timestamp + ".csv")
		if err != nil {
			log.Fatalf("ERROR - Creating file - %s\n", err)
		}
		defer iloLabelFile.Close()
		fmt.Fprintf(iloUmwlFile, "hostname,ips,os_type\r\n")
		fmt.Fprintf(iloLabelFile, "role,app,env,loc,ips\r\n")
	}

	// Set an initial reference for GAT import to increment
	ref := 0

	// Iterate through final matches
	sort.Slice(matches, func(i, j int) bool { return matches[i].matchStatus < matches[j].matchStatus })
	for _, m := range matches {

		// Check if it's a workload or IP address based off the host name
		var status string
		switch {
		case m.matchStatus == 0:
			status = "Existing workload matched to verify/assign labels."

		case m.matchStatus == 1:
			status = "IP address matched to create/label UMWL"

		case m.matchStatus == 2:
			status = "Existing Workload with no match."
		}

		// Write to the default CSV if UMWL, Matched excisting workload and exclude flag not set, non-matched existing workload and include flag not set.
		if m.matchStatus == 1 || (m.matchStatus == 0 && !exclWLs) || (m.matchStatus == 2 && nonMatchIncl) {
			fmt.Fprintf(defaultFile, "%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\r\n", m.ipAddress, m.hostname, status, m.eRole, m.eApp, m.eEnv, m.eLoc, m.role, m.app, m.env, m.loc, m.reason)
		}

		// Write GAT CSV if requested
		if gat {
			// Write UMWLs
			if m.matchStatus == 1 {
				ref++
				fmt.Fprintf(gatUmwlFile, "%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\r\n", m.hostname, "", m.role, m.app, m.env, m.loc, m.hostname, "", "", "", m.ipAddress, "eth0:"+m.ipAddress, "gat-import", "ref-"+strconv.Itoa(ref))
			}
			// Write existing workloads labels if flag isn't set
			if m.matchStatus == 0 && !exclWLs {
				fmt.Fprintf(gatLabelFile, "%s,%s,%s,%s,%s,%s\r\n", m.hostname, m.role, m.app, m.env, m.loc, m.wlHref)
			}
		}

		// Write ILO CSV if requested
		if ilo {
			// Write the UMWLs
			if m.matchStatus == 1 {
				fmt.Fprintf(iloUmwlFile, "%s,%s,%s\r\n", m.hostname, m.ipAddress, "")
				fmt.Fprintf(iloLabelFile, "%s,%s,%s,%s,%s\r\n", m.role, m.app, m.env, m.loc, m.ipAddress)
			}
			// Write the update labels
			if !exclWLs && m.matchStatus == 0 {
				fmt.Fprintf(iloLabelFile, "%s,%s,%s,%s,%s\r\n", m.role, m.app, m.env, m.loc, m.ipAddress)
			}
		}
	}
}
