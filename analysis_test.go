package handlers

import (
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/datastore"
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

func Must(t time.Time, err error) time.Time {
	if err != nil {
		panic(err)
	}
	return t
}

func TestMarathon(t *testing.T) {
	monday := Must(time.Parse("2006-01-02", "2018-03-05"))
	tuesday := monday.Add(24 * time.Hour)
	sunday := monday.Add(6 * 24 * time.Hour)
	nextMonday := monday.Add(7 * 24 * time.Hour)

	morning := 8 * time.Hour
	afternoon := 18 * time.Hour

	short := 10.0
	long := 20.0

	tcs := []struct {
		activities []*strava.ActivitySummary
		summaries  []WeekSummary
	}{
		{
			activities: []*strava.ActivitySummary{
				run(monday.Add(morning), 20*time.Minute, short),
				run(tuesday.Add(morning), 10*time.Minute, short),
				run(sunday.Add(afternoon), 22*time.Minute, long),
			},
			summaries: []WeekSummary{{3, (20 + 10 + 22) * time.Minute, short + short + long}},
		},
		{
			activities: nil,
			summaries:  nil,
		},
		{
			activities: []*strava.ActivitySummary{
				run(monday.Add(morning), 20*time.Minute, short),
				run(nextMonday.Add(morning), 21*time.Minute, long),
			},
			summaries: []WeekSummary{
				{1, 20 * time.Minute, short},
				{1, 21 * time.Minute, long},
			},
		},
		{
			activities: []*strava.ActivitySummary{
				run(monday.Add(morning), 20*time.Minute, short),
				run(sunday.Add(afternoon), 10*time.Minute, long),
				run(nextMonday.Add(morning), 21*time.Minute, long),
			},
			summaries: []WeekSummary{
				{2, 30 * time.Minute, short + long},
				{1, 21 * time.Minute, long},
			},
		},
	}

	for _, tc := range tcs {
		mt := ComputeMarathonTracking(tc.activities)
		expected := &MarathonTracking{tc.summaries}
		if !reflect.DeepEqual(mt, expected) {
			t.Errorf("Expected %v, but got %v", expected, mt)
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
