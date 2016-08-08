package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/hpcloud/tail"
)

const banMessage = "You have been banned on suspicion of proxy use.  If you believe this is in error, please contact the administrators."

var config *Config

var ipIntel = NewIpIntel()

var reTimestamp = regexp.MustCompile(`^[0-9:;\[\]]+ `)
var reConnect = regexp.MustCompile(`Connect \(v[0-9.]+\): ([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)`)

func addBan(ip string, score float64) error {
	file, err := os.OpenFile(config.Banlist, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		return err
	}

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			// Ban does not exist...
			break
		} else if err != nil {
			// We got an unexpected error, bail out with an error.
			return err
		}

		if strings.HasPrefix(line, ip) {
			log.Printf("%s is greater than or equal to MinScore, already exists in banlist. (%f >= %f)", ip, score, config.MinScore)
			return nil
		}
	}

	// Ban does not exist, append it.
	_, err = file.WriteString(fmt.Sprintf("%s:%s\n", ip, banMessage))
	if err != nil {
		return err
	}

	log.Printf("%s is greater than or equal to MinScore, added to banlist. (%f >= %f)", ip, score, config.MinScore)
	return nil
}

func parseTail(t *tail.Tail) {
	// Goroutine-specific regexes
	recTimestamp := reTimestamp.Copy()
	recConnect := reConnect.Copy()

	// Try and find a connecting IP in every single line.
	for line := range t.Lines {
		indexes := recTimestamp.FindStringIndex(line.Text)
		var testString string
		if indexes != nil {
			testString = line.Text[indexes[1]:]
		} else {
			testString = line.Text[:]
		}

		cGroups := recConnect.FindStringSubmatch(testString)
		if cGroups == nil {
			continue
		}
		ip := cGroups[1]

		// Get IP Intel on given IP.
		score, _, err := ipIntel.GetScore(ip)
		if err != nil {
			log.Printf("ipIntel error: %#v", err)
			continue
		}

		// Don't add to banlist unless we meet the minimum score.
		if score < config.MinScore {
			log.Printf("%s is less than MinScore. (%f < %f)", ip, score, config.MinScore)
			continue
		}

		err = addBan(ip, score)
		if err != nil {
			log.Printf("addBan error: %#v", err)
			continue
		}
	}
}

func main() {
	if len(os.Args) != 2 {
		log.Print("Missing parameter - config file")
		os.Exit(1)
	}

	var err error
	config, err = NewConfig(os.Args[1])
	if err != nil {
		log.Print(err.Error())
		os.Exit(1)
	}

	for _, arg := range config.Logfiles {
		t, err := tail.TailFile(arg, tail.Config{
			Follow: true,
			Location: &tail.SeekInfo{
				Offset: 0,
				Whence: os.SEEK_END,
			},
			MustExist: true,
		})
		if err != nil {
			log.Fatal(err)
		}

		go parseTail(t)
	}

	select {}
}
