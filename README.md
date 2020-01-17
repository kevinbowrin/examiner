Examiner
==============

Examiner is a cli tool which exports how many minutes off schedule an OCTranspo expected arrival time is, for a sample of future stops that day. 

Example output:

```
"Request Time","Route Number","Headsign","Stop Code","Stop Lat","Stop Lon","Minutes Off Schedule","Adjustment Age"
"2020-01-17 16:29:00 +0000 UTC","173","Barrhaven Centre","2824","45.281574","-75.756909","7","0.64"
"2020-01-17 16:29:00 +0000 UTC","81","Clyde","7371","45.401018","-75.741077","0","0.57"
"2020-01-17 16:29:00 +0000 UTC","49","Elmvale","7255","45.394094","-75.647411","38","0.64"
"2020-01-17 16:29:00 +0000 UTC","7","St-Laurent","7595","45.428924","-75.685111","-1","0.47"
"2020-01-17 16:29:00 +0000 UTC","161","Bridlewood","2064","45.284383","-75.866117","30","0.31"
"2020-01-17 16:29:00 +0000 UTC","7","St-Laurent","8725","45.448636","-75.651281","-2","0.39"
"2020-01-17 16:29:00 +0000 UTC","88","Hurdman","3033","45.392725","-75.669233","3","0.54"
"2020-01-17 16:29:00 +0000 UTC","28","Blair","2608","45.436114","-75.554368","0","0.31"
"2020-01-17 16:29:00 +0000 UTC","28","Blair","2607","45.437018","-75.554877","0","0.47"
"2020-01-17 16:29:00 +0000 UTC","33","Portobello","1747","45.478619","-75.493847","unavailable","unavailable"
```
