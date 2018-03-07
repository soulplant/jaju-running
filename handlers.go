package handlers

import (
	"errors"
	"flag"
	"html/template"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/strava/go.strava"

	"context"

	"cloud.google.com/go/datastore"
	"google.golang.org/appengine"
	"google.golang.org/appengine/urlfetch"
)

// The GCP project to connect to.
var projectId = os.Getenv("PROJECT_ID")

// The namespace to store all entities in datastore in.
var namespace = os.Getenv("NAMESPACE")

// Password for the site.
var basicAuthUser = os.Getenv("BASIC_AUTH_USER")
var basicAuthPass = os.Getenv("BASIC_AUTH_PASS")

// User represents a Strava user who has authorised access to their data.
type User struct {
	FirstName   string
	LastName    string
	StravaToken string
}

// Config is stored in datastore, entity type "Config", name "config".
type Config struct {
	// Identifies this app.
	StravaClientId string
	// Secret for authenticating us to Strava.
	StravaClientSecret string
}

func configKey() *datastore.Key {
	r := datastore.NameKey("Config", "config", nil)
	r.Namespace = namespace
	return r
}

var errConfigNotFound = errors.New("Config not found")

// fetchConfig retrieves the config from datastore.
func fetchConfig(ctx context.Context, c *datastore.Client) (*Config, error) {
	config := Config{}
	err := c.Get(ctx, configKey(), &config)
	if err != nil {
		return nil, errConfigNotFound
	}
	return &config, nil
}

// dsApi is our interface to storage and strava.
type dsApi struct {
	ctx context.Context
	ds  *datastore.Client
}

// NewDatastore creates a datastore client from an appengine context.
func NewDatastore(ctx context.Context) *datastore.Client {
	client, err := datastore.NewClient(ctx, projectId)
	if err != nil {
		panic(err)
	}

	return client
}

// NewStravaClient creates a strava client using the given access token.
func NewStravaClient(ctx context.Context, accessToken string) *strava.Client {
	return strava.NewClient(accessToken, urlfetch.Client(ctx))
}

// RegisterNewUser saves a new user's Strava token and basic details in the datastore.
func RegisterNewUser(ctx context.Context, ds *datastore.Client, auth *strava.AuthorizationResponse) (*User, error) {
	// Register new user.
	user := User{
		FirstName:   auth.Athlete.FirstName,
		LastName:    auth.Athlete.LastName,
		StravaToken: auth.AccessToken,
	}
	_, err := ds.Put(ctx, userKey(auth.Athlete.Id), &user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Summary of runs done in a week.
type WeekSummary struct {
	// How many runs were done this week.
	Count int

	// Time spent running.
	Time time.Duration

	// How much distance was covered.
	Distance float64
}

// Summary of weekly marathon training.
type MarathonTracking struct {
	weeks []WeekSummary
}

// ComputeMarathonTracking summarises the input activities into the weekly marathon tracking stats.
// Output will be sorted by week.
func ComputeMarathonTracking(activities []*strava.ActivitySummary) *MarathonTracking {
	if len(activities) == 0 {
		return &MarathonTracking{}
	}
	acts := make([]*strava.ActivitySummary, len(activities))
	copy(acts, activities)
	sort.Slice(acts, func(i, j int) bool {
		return acts[i].StartDate.Before(acts[j].StartDate)
	})
	firstRunDate := acts[0].StartDate
	curYear, curWeek := firstRunDate.ISOWeek()

	var weeks [][]*strava.ActivitySummary
	var weekActs []*strava.ActivitySummary

	for _, a := range acts {
		year, week := a.StartDate.ISOWeek()
		if curYear == year && curWeek == week {
			weekActs = append(weekActs, a)
		} else {
			weeks = append(weeks, weekActs)
			weekActs = []*strava.ActivitySummary{a}
			curYear = year
			curWeek = week
		}
	}
	if weekActs != nil {
		weeks = append(weeks, weekActs)
	}
	var sums []WeekSummary
	for _, w := range weeks {
		var dist float64
		for _, a := range w {
			dist += a.Distance
		}
		elapsed := 0
		for _, a := range w {
			elapsed += a.ElapsedTime
		}
		sum := WeekSummary{
			Count:    len(w),
			Distance: dist,
			Time:     time.Duration(elapsed) * time.Second,
		}
		sums = append(sums, sum)
	}
	return &MarathonTracking{weeks: sums}
}

func FetchMarathonTrackingForUser(ctx context.Context, s *strava.Client) (*MarathonTracking, error) {
	as, err := strava.NewCurrentAthleteService(s).ListActivities().Do()
	if err != nil {
		return nil, err
	}
	return ComputeMarathonTracking(as), nil
}

// userKey is a key based on strava's athlete id.
func userKey(id int64) *datastore.Key {
	return datastore.IDKey("User", id, nil)
}

func init() {
	clientId, err := strconv.Atoi(os.Getenv("STRAVA_CLIENT_ID"))
	if err != nil {
		// panic(errors.New("STRAVA_CLIENT_ID not set"))
	}
	strava.ClientId = clientId
	stravaClientSecret := os.Getenv("STRAVA_CLIENT_SECRET")
	if stravaClientSecret == "" {
		// panic(errors.New("STRAVA_CLIENT_SECRET not set"))
	}
	strava.ClientSecret = stravaClientSecret

	flag.Parse()
	auth := strava.OAuthAuthenticator{
		CallbackURL: "/oauth_callback",
		RequestClientGenerator: func(r *http.Request) *http.Client {
			return urlfetch.Client(appengine.NewContext(r))
		},
	}
	http.HandleFunc(auth.CallbackURL, auth.HandlerFunc(func(auth *strava.AuthorizationResponse, w http.ResponseWriter, r *http.Request) {
		ctx := appengine.NewContext(r)
		ds := NewDatastore(ctx)
		_, err := RegisterNewUser(ctx, ds, auth)
		if err != nil {
			panic(err)
		}
		http.Redirect(w, r, "/", http.StatusFound)
	}, func(err error, w http.ResponseWriter, r *http.Request) {
		panic(err)
	}))

	tpl := template.Must(template.ParseFiles("main.html.tpl"))
	http.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		err := tpl.Execute(res, struct{ Message string }{"hello, world " + namespace})
		if err != nil {
			panic(err)
		}
	})
}