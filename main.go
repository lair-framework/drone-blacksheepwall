package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/lair-framework/go-lair"
	"github.com/tomsteele/blacksheepwall/bsw"
)

const (
	version = "2.0.0"
	tool    = "blacksheepwall"
	usage   = `
Usage:
  drone-blacksheepwall <id> <filename>
  export LAIR_ID=<id>; drone-blacksheepwall <filename>
Options:
  -v              show version and exit
  -h              show usage and exit
  -k              allow insecure SSL connections
  -force-ports    disable data protection in the API server for excessive ports
  -tags           a comma separated list of tags to add to every host that is imported
`
)

func main() {
	showVersion := flag.Bool("v", false, "")
	insecureSSL := flag.Bool("k", false, "")
	forcePorts := flag.Bool("force-ports", false, "")
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
	log.Println(lairPID, filename, *insecureSSL, *forcePorts, *tags)
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatalf("Fatal: Could not open file. Error %s", err.Error())
	}
	hostTags := strings.Split(*tags, ",")
	tagSet := map[string]bool{}
	bResults := bsw.Results{}
	if err := json.Unmarshal(data, bResults); err != nil {
		log.Fatalf("Fatal: Could not parse JSON. Error %s", err.Error())
	}
	bNotFound := map[string]bool{}
	// Get this from API
	exproject := lair.Project{}
	project := lair.Project{}

	project.Tool = tool
	project.Commands = append(project.Commands, lair.Command{
		Tool: tool,
	})
	for _, result := range bResults {
		found := false
		for _, h := range exproject.Hosts {
			if result.IP == h.IPv4 {
				h.Hostnames = append(h.Hostnames, result.Hostname)
				h.LastModifiedBy = tool
				found = true
				if _, ok := tagSet[h.IPv4]; !ok {
					tagSet[h.IPv4] = true
					h.Tags = append(h.Tags, hostTags...)
				}
			}
			if !found {
				bNotFound[result.IP] = true
			}
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
			Tags:           h.Tags,
			Hostnames:      h.Hostnames,
		})
	}
	// upload project to api
	if len(bNotFound) > 0 {
		log.Println("Info: The following hosts had hostnames but could not be imported because they do not exist in lair")
	}
	for k := range bNotFound {
		fmt.Println(k)
	}
	log.Println("Success: Operation completed successfully")
}
