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
	"strconv"
	"strings"
	"sync"
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

type routetime struct {
	route_short_name string
	route_id         string
	direction_id     string
	trip_id          string
	trip_headsign    string
	arrival_time     string
	stop_code        string
}

const (
	// EnvPrefix is the prefix for the environment variables.
	EnvPrefix string = "EXAMINER_"
)

var (
	id  = flag.String("id", "", "Application ID")
	key = flag.String("key", "", "Application Key")
	db  = flag.String("db", "gtfs.db", "The file path for the sqlite3 db of GTFS data")
	v   = flag.Bool("v", false, "Verbose output")
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

	c := gooctranspoapi.NewConnection(*id, *key)

	var wg sync.WaitGroup

	addedToQueue := 0

	fmt.Println("\"Route Number\", \"Headsign\", \"Stop Code\", \"Minutes Off Schedule\", \"Adjustment Age\"")

	for _, stopTime := range sample {

		arrivalDay := time.Now()

		hourAsInt, err := strconv.Atoi(stopTime.arrival_time[:2])
		if err != nil {
			log.Fatal(err)
		}

		if hourAsInt > 23 {
			hourAsInt = hourAsInt - 24
			stopTime.arrival_time = fmt.Sprintf("0%v:%v", hourAsInt, stopTime.arrival_time[3:])
			arrivalDay = arrivalDay.AddDate(0, 0, 1)
		}

		zone, _ := arrivalDay.Zone()

		arrival, err := time.Parse("2006-01-02 15:04:05 MST", arrivalDay.Format("2006-01-02 ")+stopTime.arrival_time+" "+zone)
		if err != nil {
			log.Fatal(err)
		}

		checkAt := arrival.Add(-time.Minute * 5)

		if *v {
			fmt.Printf("%v, %v, %v, %v, check at %v\n", stopTime.route_short_name, stopTime.trip_headsign, stopTime.stop_code, stopTime.arrival_time, checkAt)
		}

		if time.Now().Before(checkAt) {

			addedToQueue++

			wg.Add(1)
			// Launch a goroutine to fetch the URL.
			go func(waitUntil time.Time, stopTime routetime) {
				defer wg.Done()

				waitDur := waitUntil.Sub(time.Now())
				time.Sleep(waitDur)

				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				nextTrips, err := c.GetNextTripsForStop(ctx, stopTime.route_short_name, stopTime.stop_code)
				if err != nil {
					log.Printf("Error: %v\n", err)
					log.Printf("Error: StopTime - %#v\n", stopTime)
					return
				}

				for _, routedirection := range nextTrips.RouteDirections {
					if strings.TrimSpace(routedirection.RouteNo) == strings.TrimSpace(stopTime.route_short_name) &&
						strings.TrimSpace(routedirection.RouteLabel) == strings.TrimSpace(stopTime.trip_headsign) {
						if len(routedirection.Trips) > 0 {

							if *v {
								log.Printf("StopTime: %#v\n", stopTime)
								log.Printf("RouteDirection: %#v\n", routedirection)
								log.Printf("Trips: %#v\n", routedirection.Trips)
							}

							trip := routedirection.Trips[0]
							if trip.AdjustmentAge > 0 {
								minus5 := trip.AdjustedScheduleTime - 5
								fmt.Printf("%v,\"%v\",%v,%v,%v\n", stopTime.route_short_name, stopTime.trip_headsign, stopTime.stop_code, minus5, trip.AdjustmentAge)
							} else {
								fmt.Printf("%v,\"%v\",%v,%v,%v\n", stopTime.route_short_name, stopTime.trip_headsign, stopTime.stop_code, "unavailable", "unavailable")
							}

						}
					}
				}

			}(checkAt, stopTime)
		}

	}

	if *v {
		log.Printf("%v future API calls added to queue...\n", addedToQueue)
	}

	wg.Wait()
}

func getSampleOfStopTimes(dbfilepath string) ([]routetime, error) {

	routetimes := []routetime{}

	todayServices, err := getTodaysServices(dbfilepath)
	if err != nil {
		return []routetime{}, err
	}

	db, err := sql.Open("sqlite3", dbfilepath)
	if err != nil {
		return routetimes, err
	}
	defer db.Close()

	ts := make([]interface{}, len(todayServices))
	for i, v := range todayServices {
		ts[i] = v
	}

	routetimequery := `
        SELECT routes.route_short_name, trips.route_id, trips.direction_id, trips.trip_id, trips.trip_headsign, stop_times.arrival_time, stops.stop_code
        FROM trips
        LEFT JOIN stop_times
        ON stop_times.trip_id = trips.trip_id
        LEFT JOIN routes
        ON routes.route_id = trips.route_id 
        LEFT JOIN stops 
        ON stops.stop_id = stop_times.stop_id
        WHERE trips.service_id IN ` + "(?" + strings.Repeat(",?", len(ts)-1) + ")" + `
        AND stop_times.stop_sequence != 1
        AND stop_times.pickup_type == 0
        ORDER BY RANDOM() LIMIT 10000`

	routetimerows, err := db.Query(routetimequery, ts...)
	if err != nil {
		return routetimes, err
	}
	defer routetimerows.Close()
	for routetimerows.Next() {
		var curRow routetime
		err = routetimerows.Scan(&curRow.route_short_name, &curRow.route_id, &curRow.direction_id, &curRow.trip_id, &curRow.trip_headsign, &curRow.arrival_time, &curRow.stop_code)
		if err != nil {
			return routetimes, err
		}
		routetimes = append(routetimes, curRow)
	}
	err = routetimerows.Err()
	if err != nil {
		return routetimes, err
	}

	return routetimes, nil

}

func getTodaysServices(dbfilepath string) ([]string, error) {
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
		return []string{}, err
	}
	defer calRows.Close()
	for calRows.Next() {
		var curRow calendarrow
		err = calRows.Scan(&curRow.service_id, &curRow.monday, &curRow.tuesday, &curRow.wednesday, &curRow.thursday, &curRow.friday, &curRow.saturday, &curRow.sunday, &curRow.start_date, &curRow.end_date)
		if err != nil {
			return []string{}, err
		}

		start, err := time.ParseInLocation("20060102", curRow.start_date, today.Location())
		if err != nil {
			return []string{}, err
		}

		end, err := time.ParseInLocation("20060102", curRow.end_date, today.Location())
		if err != nil {
			return []string{}, err
		}
		end = end.Add(time.Hour * 23)
		end = end.Add(time.Minute * 59)
		end = end.Add(time.Second * 59)

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
		return []string{}, err
	}

	exceptions, err := db.Query("SELECT service_id,date,exception_type FROM calendar_dates WHERE date = ?", today.Format("20060102"))
	if err != nil {
		return []string{}, err
	}
	defer exceptions.Close()
	for exceptions.Next() {
		var curRow calendadaterow
		err = exceptions.Scan(&curRow.service_id, &curRow.date, &curRow.exception_type)
		if err != nil {
			return []string{}, err
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
		return []string{}, err
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
