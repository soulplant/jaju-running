package handlers

import (
	"bytes"
	"fmt"
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
		Athlete:     strava.AthleteSummary{FirstName: "james"},
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

var saturday = Must(time.Parse("2006-01-02", "2018-03-03"))
var monday = Must(time.Parse("2006-01-02", "2018-03-05"))
var tuesday = monday.Add(24 * time.Hour)
var nextSaturday = saturday.Add(7 * 24 * time.Hour)

const morning = 8 * time.Hour
const afternoon = 18 * time.Hour

var week1 = saturday
var week2 = saturday.Add(7 * 24 * time.Hour)

const short = 10.0
const long = 20.0

func TestMarathon(t *testing.T) {

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
		mt := ComputeWeeklySummaries(tc.activities)
		expected := tc.summaries
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

type fakeFetcher struct {
	acts []*strava.ActivitySummary
}

func (f fakeFetcher) FetchActivities(token string) ([]*strava.ActivitySummary, error) {
	// return nil, errors.New("hi")
	return f.acts, nil
}

func TestPrepareTable(t *testing.T) {
	f := fakeFetcher{
		acts: []*strava.ActivitySummary{run(saturday, 1*time.Hour, short)},
	}
	umt, err := FetchUserHistory([]User{{"not-read", "k", "abc123"}}, f)
	if err != nil {
		t.Fatal(err)
	}
	if umt[0].Name != "james" {
		t.Error("Expected name to be james")
	}
}

func process(acts ...*strava.ActivitySummary) []*UserMarathonTracking {
	f := fakeFetcher{
		acts: acts,
	}
	umt, err := FetchUserHistory([]User{{"not-read", "k", "abc123"}}, f)
	if err != nil {
		panic(err)
	}
	return umt
}

func TestTemplate(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	umt := process(run(saturday, 1*time.Hour, 1), run(nextSaturday, 1*time.Hour, 2))
	err := mainTpl.Execute(buf, MainTplArgs{
		Umt:         umt,
		ClientId:    fmt.Sprintf("%d", 1234),
		RedirectUri: "localhost:1234/oauth_callback",
	})
	if err != nil {
		t.Logf("got an error: %s", err)
		t.Fail()
	}
	t.Logf("%s", buf.String())
	t.Fail()
}
