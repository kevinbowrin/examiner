#! /usr/bin/env bash

curpwd=$PWD
cd /tmp
wget http://www.octranspo1.com/files/google_transit.zip
unzip google_transit.zip
dbfilename=gtfs-$(date +%F).db
sqlite3 $dbfilename ".mode csv" ".import agency.txt agency"
sqlite3 $dbfilename ".mode csv" ".import calendar.txt calendar"
sqlite3 $dbfilename ".mode csv" ".import routes.txt routes"
sqlite3 $dbfilename ".mode csv" ".import stop_times.txt stop_times"
sqlite3 $dbfilename ".mode csv" ".import calendar_dates.txt calendar_dates"
sqlite3 $dbfilename ".mode csv" ".import stops.txt stops"
sqlite3 $dbfilename ".mode csv" ".import trips.txt trips"
mv /tmp/$dbfilename $curpwd/gtfs.db
rm google_transit.zip
rm agency.txt
rm calendar.txt
rm routes.txt
rm stop_times.txt
rm calendar_dates.txt
rm stops.txt
rm trips.txt 
