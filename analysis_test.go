package handlers

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	strava "github.com/strava/go.strava"
	context "golang.org/x/net/context"
	"google.golang.org/appengine"
	"google.golang.org/appengine/aetest"
)

// run creates a run activity.
func run(time time.Time, duration time.Duration, distance float64) *strava.ActivitySummary {
	return &strava.ActivitySummary{
		Athlete:     strava.AthleteSummary{FirstName: "james"},
		Type:        strava.ActivityTypes.Run,
		StartDate:   time,
		ElapsedTime: int(duration.Seconds()),
		Distance:    distance,
	}
}

// ride creates a ride activity.
func ride(time time.Time, duration time.Duration, distance float64) *strava.ActivitySummary {
	return &strava.ActivitySummary{
		Type:        strava.ActivityTypes.Ride,
		StartDate:   time,
		ElapsedTime: int(duration.Seconds()),
		Distance:    distance,
	}
}

// Must returns its input time or panics if there's an error.
func Must(t time.Time, err error) time.Time {
	if err != nil {
		panic(err)
	}
	return t
}

var (
	saturday     = Must(time.Parse("2006-01-02", "2018-03-03"))
	monday       = Must(time.Parse("2006-01-02", "2018-03-05"))
	tuesday      = monday.Add(24 * time.Hour)
	nextSaturday = saturday.Add(7 * 24 * time.Hour)
)

const (
	morning   = 8 * time.Hour
	afternoon = 18 * time.Hour
)

// Start dates for weeks.
var (
	week1 = saturday
	week2 = saturday.Add(7 * 24 * time.Hour)
)

// Run durations in metres.
const (
	short = 5.6 * 1000
	long  = 11.2 * 1000
)

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
		{
			message: "only rides in a week, then runs next week",
			activities: []*strava.ActivitySummary{
				ride(saturday.Add(morning), 20*time.Minute, short),
				run(nextSaturday.Add(morning), 20*time.Minute, short),
			},
			summaries: []WeekSummary{
				{week2, 1, 20 * time.Minute, short},
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

// makeAuth returns a synthesised Strava auth response.
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
	inst, err := aetest.NewInstance(&aetest.Options{
		StronglyConsistentDatastore: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	defer inst.Close() // nolint: errcheck
	req, err := inst.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := appengine.NewContext(req)
	users, err := GetUsers(ctx)
	if err != nil {
		t.Fatalf("Couldn't read users: %s", err)
	}
	if len(users) != 0 {
		t.Errorf("Expected 0 users, got %d", len(users))
	}
	auth := makeAuth("abc-123", "james", "k", 1234)
	u, err := RegisterNewUser(ctx, auth)
	if err != nil {
		t.Fatalf("Failed to register user %s", err)
	}
	expectedU := &User{"james", "k", "abc-123"}
	if !reflect.DeepEqual(u, expectedU) {
		t.Fatalf("Expected %v got %v", u, expectedU)
	}
	// Note, this only works because datastore is run in strongly consistent mode.
	users, err = GetUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Errorf("Expected 1 user, got %d", len(users))
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

type fakeFetcher struct {
	acts []*strava.ActivitySummary
}

func (f fakeFetcher) FetchActivities(token string) ([]*strava.ActivitySummary, error) {
	return f.acts, nil
}

func TestPrepareTable(t *testing.T) {
	f := stubFetcher(map[string][]*strava.ActivitySummary{
		"abc123": []*strava.ActivitySummary{run(saturday, 1*time.Hour, short)},
	})
	umt, err := FetchUserHistory(context.Background(), []User{{"james2", "k", "abc123"}}, f)
	if err != nil {
		t.Fatal(err)
	}
	if umt[0].Name != "james2" {
		t.Error("Expected name to be james2, but was" + umt[0].Name)
	}
}

type stubFetcher map[string][]*strava.ActivitySummary

func (sf stubFetcher) FetchActivities(token string) ([]*strava.ActivitySummary, error) {
	acts, ok := sf[token]
	if !ok {
		return nil, errors.New("not found")
	}
	fmt.Printf("Returning '%s': %v", token, acts)
	return acts, nil
}

func TestFetchUsersActivity(t *testing.T) {
	actsA := []*strava.ActivitySummary{run(saturday, 1*time.Hour, short)}
	actsB := []*strava.ActivitySummary{run(monday, 1*time.Hour, short)}
	m := map[string][]*strava.ActivitySummary{
		"a": actsA,
		"b": actsB,
	}
	f := stubFetcher(m)
	_ = stubFetcher(map[string][]*strava.ActivitySummary{
		"a": actsA,
	})

	cases := []struct {
		users []User
		acts  [][]*strava.ActivitySummary
		fail  bool
	}{
		{
			users: []User{{StravaToken: "a"}},
			acts:  [][]*strava.ActivitySummary{actsA},
		},
		{
			users: []User{{StravaToken: "a"}, {StravaToken: "b"}},
			acts:  [][]*strava.ActivitySummary{actsA, actsB},
		},
		{
			users: []User{{StravaToken: "b"}, {StravaToken: "b"}},
			acts:  [][]*strava.ActivitySummary{actsB, actsB},
		},
		{
			users: []User{{StravaToken: "b"}, {StravaToken: "a"}},
			acts:  [][]*strava.ActivitySummary{actsB, actsA},
		},
		{
			users: []User{{StravaToken: "a"}, {StravaToken: "mystery"}},
			fail:  true,
		},
		{
			users: []User{{StravaToken: "mystery"}},
			fail:  true,
		},
		{
			users: []User{{StravaToken: "mystery"}, {StravaToken: "other-mystery"}},
			fail:  true,
		},
	}
	for _, c := range cases {
		acts, err := FetchUsersActivity(c.users, f)
		if c.fail && err == nil {
			t.Error("Expected a failure, but got nil error")
		}
		if !reflect.DeepEqual(acts, c.acts) {
			t.Errorf("Expected acts %v, got %v", c.acts, acts)
		}
	}
}
