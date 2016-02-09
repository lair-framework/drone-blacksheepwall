package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/lair-framework/api-server/client"
	"github.com/lair-framework/go-lair"
	"github.com/tomsteele/blacksheepwall/bsw"
)

const (
	version = "2.1.0"
	tool    = "drone-blacksheepwall"
	usage   = `
Parses a blacksheepwall JSON file into a lair project.

Usage:
  drone-blacksheepwall [options] <id> <filename>
  export LAIR_ID=<id>; drone-blacksheepwall [options] <filename>
Options:
  -v              show version and exit
  -h              show usage and exit
  -k              allow insecure SSL connections
  -force-hosts    import all hosts into Lair, default behaviour is to only import
                  hostnames for hosts that already exist in a project
  -force-ports    disable data protection in the API server for excessive ports
  -tags           a comma separated list of tags to add to every host that is imported
`
)

func main() {
	showVersion := flag.Bool("v", false, "")
	insecureSSL := flag.Bool("k", false, "")
	forcePorts := flag.Bool("force-ports", false, "")
	forceHosts := flag.Bool("force-hosts", false, "")
	tags := flag.String("tags", "", "")
	flag.Usage = func() {
		fmt.Println(usage)
	}
	flag.Parse()
	if *showVersion {
		log.Println(version)
		os.Exit(0)
	}
	lairURL := os.Getenv("LAIR_API_SERVER")
	if lairURL == "" {
		log.Fatal("Fatal: Missing LAIR_API_SERVER environment variable")
	}
	lairPID := os.Getenv("LAIR_ID")
	var filename string
	switch len(flag.Args()) {
	case 2:
		lairPID = flag.Arg(0)
		filename = flag.Arg(1)
	case 1:
		filename = flag.Arg(0)
	default:
		log.Fatal("Fatal: Missing required argument")
	}

	u, err := url.Parse(lairURL)
	if err != nil {
		log.Fatalf("Fatal: Error parsing LAIR_API_SERVER URL. Error %s", err.Error())
	}
	if u.User == nil {
		log.Fatal("Fatal: Missing username and/or password")
	}
	user := u.User.Username()
	pass, _ := u.User.Password()
	if user == "" || pass == "" {
		log.Fatal("Fatal: Missing username and/or password")
	}
	c, err := client.New(&client.COptions{
		User:               user,
		Password:           pass,
		Host:               u.Host,
		Scheme:             u.Scheme,
		InsecureSkipVerify: *insecureSSL,
	})
	if err != nil {
		log.Fatalf("Fatal: Error setting up client: Error %s", err.Error())
	}
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatalf("Fatal: Could not open file. Error %s", err.Error())
	}
	hostTags := []string{}
	if *tags != "" {
		hostTags = strings.Split(*tags, ",")
	}
	tagSet := map[string]bool{}
	bResults := bsw.Results{}
	if err := json.Unmarshal(data, &bResults); err != nil {
		log.Fatalf("Fatal: Could not parse JSON. Error %s", err.Error())
	}
	bNotFound := map[string]bsw.Results{}

	exproject, err := c.ExportProject(lairPID)
	if err != nil {
		log.Fatalf("Fatal: Unable to export project. Error %s", err.Error())
	}

	project := &lair.Project{
		ID:   lairPID,
		Tool: tool,
		Commands: []lair.Command{lair.Command{
			Tool: tool,
		}},
	}

	for _, result := range bResults {
		found := false
		for i := range exproject.Hosts {
			h := exproject.Hosts[i]
			if result.IP == h.IPv4 {
				exproject.Hosts[i].Hostnames = append(exproject.Hosts[i].Hostnames, result.Hostname)
				exproject.Hosts[i].LastModifiedBy = tool
				found = true
				if _, ok := tagSet[h.IPv4]; !ok {
					tagSet[h.IPv4] = true
					exproject.Hosts[i].Tags = append(exproject.Hosts[i].Tags, hostTags...)
				}
			}
		}
		if !found {
			bNotFound[result.IP] = append(bNotFound[result.IP], result)
		}
	}

	for _, h := range exproject.Hosts {
		project.Hosts = append(project.Hosts, lair.Host{
			IPv4:           h.IPv4,
			LongIPv4Addr:   h.LongIPv4Addr,
			IsFlagged:      h.IsFlagged,
			LastModifiedBy: h.LastModifiedBy,
			MAC:            h.MAC,
			OS:             h.OS,
			Status:         h.Status,
			StatusMessage:  h.StatusMessage,
			Tags:           hostTags,
			Hostnames:      h.Hostnames,
		})
	}

	if *forceHosts {
		for ip, results := range bNotFound {
			hostnames := []string{}
			for _, r := range results {
				hostnames = append(hostnames, r.Hostname)
			}
			project.Hosts = append(project.Hosts, lair.Host{
				IPv4:      ip,
				Hostnames: hostnames,
			})
		}
	}

	res, err := c.ImportProject(&client.DOptions{ForcePorts: *forcePorts}, project)
	if err != nil {
		log.Fatalf("Fatal: Unable to import project. Error %s", err)
	}

	defer res.Body.Close()
	droneRes := &client.Response{}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatalf("Fatal: Error %s", err.Error())
	}
	if err := json.Unmarshal(body, droneRes); err != nil {
		log.Fatalf("Fatal: Could not unmarshal JSON. Error %s", err.Error())
	}
	if droneRes.Status == "Error" {
		log.Fatalf("Fatal: Import failed. Error %s", droneRes.Message)
	}
	if len(bNotFound) > 0 {
		if *forceHosts {
			log.Println("Info: The following hosts had hostnames and were forced to import into lair")
		} else {
			log.Println("Info: The following hosts had hostnames but could not be imported because they do not exist in lair")
		}
	}
	for k := range bNotFound {
		fmt.Println(k)
	}
	log.Println("Success: Operation completed successfully")
}
