package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"github.com/transitreport/gooctranspoapi"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type calendarrow struct {
	service_id string
	monday     string
	tuesday    string
	wednesday  string
	thursday   string
	friday     string
	saturday   string
	sunday     string
	start_date string
	end_date   string
}

type calendadaterow struct {
	service_id     string
	date           string
	exception_type string
}

const (
	// EnvPrefix is the prefix for the environment variables.
	EnvPrefix string = "EXAMINER_"
)

var (
	id  = flag.String("id", "", "Application ID")
	key = flag.String("key", "", "Application Key")
	db  = flag.String("db", "gtfs.db", "The file path for the sqlite3 db of GTFS data")
	// A version flag, which should be overwritten when building using ldflags.
	version = "devel"
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Examiner\nVersion %v\n\n", version)
		fmt.Fprintln(os.Stderr, "A tool for analyzing transit arrival times vs scheduled arrival times.")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "  The possible environment variables:")

		flag.VisitAll(func(f *flag.Flag) {
			uppercaseName := strings.ToUpper(f.Name)
			fmt.Fprintf(os.Stderr, "  %v%v\n", EnvPrefix, uppercaseName)
		})
	}
}

func main() {
	// Process the flags.
	flag.Parse()

	// If any flags have not been set, see if there are
	// environment variables that set them.
	overrideUnsetFlagsFromEnvironmentVariables()

	// If any of the required flags are not set, exit.
	if *id == "" {
		log.Fatal("FATAL: An Application ID is required.")
	} else if *key == "" {
		log.Fatal("FATAL: An Application Key is required.")
	}

	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Could not find working directory, %v", err)
	}
	if !filepath.IsAbs(*db) {
		*db = filepath.Join(wd, *db)
	}

	sample, err := getSampleOfStopTimes(*db)
	if err != nil {
		log.Fatalf("Error creating stop times, %v", err)
	}

	for _, stopTime := range sample {
		fmt.Println(stopTime)
	}

	c := gooctranspoapi.NewConnection(*id, *key)
	data, err := c.GetGTFSAgency(context.TODO())

	log.Println(data)
	log.Println(err)
}

func getSampleOfStopTimes(dbfilepath string) ([]string, error) {
	db, err := sql.Open("sqlite3", dbfilepath)
	if err != nil {
		return []string{}, err
	}
	defer db.Close()

	today := time.Now()
	weekday := today.Weekday()
	todayServices := []string{}

	calRows, err := db.Query("SELECT service_id,monday,tuesday,wednesday,thursday,friday,saturday,sunday,start_date,end_date FROM calendar")
	if err != nil {
		log.Fatal(err)
	}
	defer calRows.Close()
	for calRows.Next() {
		var curRow calendarrow
		err = calRows.Scan(&curRow.service_id, &curRow.monday, &curRow.tuesday, &curRow.wednesday, &curRow.thursday, &curRow.friday, &curRow.saturday, &curRow.sunday, &curRow.start_date, &curRow.end_date)
		if err != nil {
			log.Fatal(err)
		}

		start, err := time.Parse("20060102", curRow.start_date)
		if err != nil {
			log.Fatal(err)
		}

		end, err := time.Parse("20060102", curRow.end_date)
		if err != nil {
			log.Fatal(err)
		}

		if today.After(start) && today.Before(end) {
			switch weekday {
			case time.Monday:
				if curRow.monday == "1" {
					todayServices = append(todayServices, curRow.service_id)
				}
			case time.Tuesday:
				if curRow.tuesday == "1" {
					todayServices = append(todayServices, curRow.service_id)
				}
			case time.Wednesday:
				if curRow.wednesday == "1" {
					todayServices = append(todayServices, curRow.service_id)
				}
			case time.Thursday:
				if curRow.thursday == "1" {
					todayServices = append(todayServices, curRow.service_id)
				}
			case time.Friday:
				if curRow.friday == "1" {
					todayServices = append(todayServices, curRow.service_id)
				}
			case time.Saturday:
				if curRow.saturday == "1" {
					todayServices = append(todayServices, curRow.service_id)
				}
			case time.Sunday:
				if curRow.sunday == "1" {
					todayServices = append(todayServices, curRow.service_id)
				}
			}
		}
	}
	err = calRows.Err()
	if err != nil {
		log.Fatal(err)
	}

	exceptions, err := db.Query("SELECT service_id,date,exception_type FROM calendar_dates WHERE date = ?", today.Format("20060102"))
	if err != nil {
		log.Fatal(err)
	}
	defer exceptions.Close()
	for exceptions.Next() {
		var curRow calendadaterow
		err = exceptions.Scan(&curRow.service_id, &curRow.date, &curRow.exception_type)
		if err != nil {
			log.Fatal(err)
		}
		if curRow.exception_type == "2" {
			for i, service_id := range todayServices {
				if service_id == curRow.service_id {
					todayServices = append(todayServices[:i], todayServices[i+1:]...)
					break
				}
			}
		}
	}
	err = exceptions.Err()
	if err != nil {
		log.Fatal(err)
	}

	return todayServices, nil
}

// If any flags are not set, use environment variables to set them.
func overrideUnsetFlagsFromEnvironmentVariables() {

	// A map of pointers to unset flags.
	listOfUnsetFlags := make(map[*flag.Flag]bool)

	// flag.Visit calls a function on "only those flags that have been set."
	// flag.VisitAll calls a function on "all flags, even those not set."
	// No way to ask for "only unset flags". So, we add all, then
	// delete the set flags.

	// First, visit all the flags, and add them to our map.
	flag.VisitAll(func(f *flag.Flag) { listOfUnsetFlags[f] = true })

	// Then delete the set flags.
	flag.Visit(func(f *flag.Flag) { delete(listOfUnsetFlags, f) })

	// Loop through our list of unset flags.
	// We don't care about the values in our map, only the keys.
	for k := range listOfUnsetFlags {

		// Build the corresponding environment variable name for each flag.
		uppercaseName := strings.ToUpper(k.Name)
		environmentVariableName := fmt.Sprintf("%v%v", EnvPrefix, uppercaseName)

		// Look for the environment variable name.
		// If found, set the flag to that value.
		// If there's a problem setting the flag value,
		// there's a serious problem we can't recover from.
		environmentVariableValue := os.Getenv(environmentVariableName)
		if environmentVariableValue != "" {
			err := k.Value.Set(environmentVariableValue)
			if err != nil {
				log.Fatalf("FATAL: Unable to set configuration option %v from environment variable %v, "+
					"which has a value of \"%v\"",
					k.Name, environmentVariableName, environmentVariableValue)
			}
		}
	}
}
