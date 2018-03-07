package handlers

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/datastore"
	"github.com/olekukonko/tablewriter"
	strava "github.com/strava/go.strava"
	context "golang.org/x/net/context"
)

func run(time time.Time, duration time.Duration, distance float64) *strava.ActivitySummary {
	return &strava.ActivitySummary{
		Type:        strava.ActivityTypes.Run,
		StartDate:   time,
		ElapsedTime: int(duration.Seconds()),
		Distance:    distance,
	}
}

func ride(time time.Time, duration time.Duration, distance float64) *strava.ActivitySummary {
	return &strava.ActivitySummary{
		Type:        strava.ActivityTypes.Ride,
		StartDate:   time,
		ElapsedTime: int(duration.Seconds()),
		Distance:    distance,
	}
}

func Must(t time.Time, err error) time.Time {
	if err != nil {
		panic(err)
	}
	return t
}

func TestMarathon(t *testing.T) {
	saturday := Must(time.Parse("2006-01-02", "2018-03-03"))
	monday := Must(time.Parse("2006-01-02", "2018-03-05"))
	tuesday := monday.Add(24 * time.Hour)
	nextSaturday := saturday.Add(7 * 24 * time.Hour)

	morning := 8 * time.Hour
	afternoon := 18 * time.Hour

	week1 := saturday
	week2 := saturday.Add(7 * 24 * time.Hour)

	short := 10.0
	long := 20.0

	tcs := []struct {
		message    string
		activities []*strava.ActivitySummary
		summaries  []WeekSummary
	}{
		{
			message: "all in the same week",
			activities: []*strava.ActivitySummary{
				run(saturday.Add(afternoon), 22*time.Minute, long),
				run(monday.Add(morning), 20*time.Minute, short),
				run(tuesday.Add(morning), 10*time.Minute, short),
			},
			summaries: []WeekSummary{{week1, 3, (20 + 10 + 22) * time.Minute, short + short + long}},
		},
		{
			message:    "no activities, no summaries",
			activities: nil,
			summaries:  nil,
		},
		{
			message: "across two weeks",
			activities: []*strava.ActivitySummary{
				run(monday.Add(morning), 20*time.Minute, short),
				run(nextSaturday.Add(morning), 21*time.Minute, long),
			},
			summaries: []WeekSummary{
				{week1, 1, 20 * time.Minute, short},
				{week2, 1, 21 * time.Minute, long},
			},
		},
		{
			message: "two in first, one in second week",
			activities: []*strava.ActivitySummary{
				run(saturday.Add(morning), 20*time.Minute, short),
				run(monday.Add(afternoon), 10*time.Minute, long),
				run(nextSaturday.Add(morning), 21*time.Minute, long),
			},
			summaries: []WeekSummary{
				{week1, 2, 30 * time.Minute, short + long},
				{week2, 1, 21 * time.Minute, long},
			},
		},
		{
			message: "rides are skipped",
			activities: []*strava.ActivitySummary{
				ride(saturday.Add(morning), 20*time.Minute, short),
			},
			summaries: nil,
		},
		{
			message: "rides are skipped 2",
			activities: []*strava.ActivitySummary{
				ride(saturday.Add(morning), 20*time.Minute, short),
				run(monday.Add(morning), 20*time.Minute, short),
			},
			summaries: []WeekSummary{
				{week1, 1, 20 * time.Minute, short},
			},
		},
	}

	for _, tc := range tcs {
		mt := ComputeMarathonTracking(tc.activities)
		expected := &MarathonTracking{tc.summaries}
		if !reflect.DeepEqual(mt, expected) {
			t.Errorf("Case '%s': Expected %v, but got %v", tc.message, expected, mt)
		}
	}
}

func makeAuth(token, fname, lname string, id int64) *strava.AuthorizationResponse {
	return &strava.AuthorizationResponse{
		AccessToken: token,
		Athlete: strava.AthleteDetailed{
			AthleteSummary: strava.AthleteSummary{
				AthleteMeta: strava.AthleteMeta{
					Id: id,
				},
				FirstName: fname,
				LastName:  lname,
			},
		},
	}
}

func TestRegisterNewUser(t *testing.T) {
	ds := InitTestDatastore(t)
	out := make([]User, 0)
	keys, err := ds.GetAll(context.Background(), datastore.NewQuery("User"), &out)
	if err != nil {
		t.Error(err)
	}
	if len(keys) != 0 {
		t.Error("Expected 0 keys, got " + string(len(keys)))
	}
	auth := makeAuth("abc-123", "james", "k", 1234)
	ctx := context.Background()
	u, err := RegisterNewUser(ctx, ds, auth)
	if err != nil {
		t.Fatalf("Failed to register user %s", err)
	}
	expectedU := &User{"james", "k", "abc-123"}
	if !reflect.DeepEqual(u, expectedU) {
		t.Fatalf("Expected %v got %v", u, expectedU)
	}
}

func TestPreviousSaturday(t *testing.T) {
	format := "2006-01-02"

	for _, tc := range []struct {
		input    string
		expected string
	}{
		{"2018-03-05", "2018-03-03"},
		{"2018-03-03", "2018-03-03"},
		{"2018-03-02", "2018-02-24"},
	} {
		id := Must(time.Parse(format, tc.input))
		ed := Must(time.Parse(format, tc.expected))
		ad := PreviousSaturday(id)
		if ed != ad {
			t.Errorf("Expected %s got %s", ed.Format(format), ad.Format(format))
		}
	}

	sat := Must(time.Parse(format, "2018-03-03"))
	satMorn := sat.Add(8 * time.Hour)
	if PreviousSaturday(satMorn) != PreviousSaturday(sat) {
		t.Error("previous saturday shouldn't change depending on the time")
	}
}

func TestFoo(t *testing.T) {
	buf := bytes.NewBufferString("")
	tw := tablewriter.NewWriter(buf)
	tw.SetHeader([]string{"Date", "Count", "Distance", "Time"})
	tw.Append([]string{"hi"})
	tw.Render()
	t.Logf(buf.String())
	t.Fail()
}
