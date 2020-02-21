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
	"io/ioutil"
	"encoding/json"
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

type RouteTime struct {
	RouteShortName string
	RouteID         string
	DirectionID     string
	TripID          string
	TripHeadsign    string
	ArrivalTime     string
	StopCode        string
	StopLat         string
	StopLon         string
}

type Export struct {
	Routetime *RouteTime
        NextTripsForStop *gooctranspoapi.NextTripsForStop
	RequestedAt time.Time
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

	stopTimes, err := getStopTimes(*db)
	if err != nil {
		log.Fatalf("Error querying for stop times, %v", err)
	}

	c := gooctranspoapi.NewConnection(*id, *key)

	var wg sync.WaitGroup

	addedToQueue := 0

	for _, stopTime := range stopTimes {

		arrivalDay := time.Now()

		hourAsInt, err := strconv.Atoi(stopTime.ArrivalTime[:2])
		if err != nil {
			log.Fatal(err)
		}

		if hourAsInt > 23 {
			hourAsInt = hourAsInt - 24
			stopTime.ArrivalTime = fmt.Sprintf("0%v:%v", hourAsInt, stopTime.ArrivalTime[3:])
			arrivalDay = arrivalDay.AddDate(0, 0, 1)
		}

		zone, _ := arrivalDay.Zone()

		arrival, err := time.Parse("2006-01-02 15:04:05 MST", arrivalDay.Format("2006-01-02 ")+stopTime.ArrivalTime+" "+zone)
		if err != nil {
			log.Fatal(err)
		}

		checkAt := arrival.Add(-time.Minute * 5)

		if *v {
			log.Printf("%v, %v, %v, %v, check at %v\n", stopTime.RouteShortName, stopTime.TripHeadsign, stopTime.StopCode, stopTime.ArrivalTime, checkAt)
		}

		if time.Now().Before(checkAt) {

			addedToQueue++

			wg.Add(1)
			// Launch a goroutine to fetch the URL.
			go func(waitUntil time.Time, stopTime RouteTime) {
				defer wg.Done()

				waitDur := waitUntil.Sub(time.Now())
				time.Sleep(waitDur)

				requestedAt := time.Now()
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				nextTrips, err := c.GetNextTripsForStop(ctx, stopTime.RouteShortName, stopTime.StopCode)
				if err != nil {
					log.Printf("Error: %v\n", err)
					log.Printf("Error: StopTime - %#v\n", stopTime)
					return
				}

				ex := Export{
					Routetime: &stopTime,
					NextTripsForStop: nextTrips,
					RequestedAt: requestedAt,
				}

				json, err := json.MarshalIndent(ex, "", " ")
				if err != nil {
					log.Printf("Error: %v\n", err)
					return
				}

				err = ioutil.WriteFile(fmt.Sprintf("%v_%v_%v.json", strings.Replace(requestedAt.Format("2006-01-02-150405.000"), ".", "", -1), stopTime.RouteShortName, stopTime.StopCode), json, 0644)

				if err != nil {
					log.Printf("Error: %v\n", err)
				}

			}(checkAt, stopTime)
		}

		if addedToQueue >= 10000 {
			break
		}

	}

	if *v {
		log.Printf("%v future API calls added to queue...\n", addedToQueue)
	}

	wg.Wait()
}

func getStopTimes(dbfilepath string) ([]RouteTime, error) {

	routetimes := []RouteTime{}

	todayServices, err := getTodaysServices(dbfilepath)
	if err != nil {
		return routetimes, err
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
        SELECT routes.route_short_name, trips.route_id, trips.direction_id, trips.trip_id, trips.trip_headsign, stop_times.arrival_time, stops.stop_code, stops.stop_lat, stops.stop_lon
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
        ORDER BY RANDOM()`

	routetimerows, err := db.Query(routetimequery, ts...)
	if err != nil {
		return routetimes, err
	}
	defer routetimerows.Close()
	for routetimerows.Next() {
		var curRow RouteTime
		err = routetimerows.Scan(&curRow.RouteShortName, &curRow.RouteID, &curRow.DirectionID, &curRow.TripID, &curRow.TripHeadsign, &curRow.ArrivalTime, &curRow.StopCode, &curRow.StopLat, &curRow.StopLon)
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
