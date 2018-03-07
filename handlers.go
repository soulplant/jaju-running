package handlers

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/olekukonko/tablewriter"
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
	// The day this week starts on.
	Date time.Time

	// How many runs were done this week.
	Count int

	// Time spent running.
	Time time.Duration

	// How much distance was covered.
	Distance float64
}

// Summary of weekly marathon training for a given user.
type UserMarathonTracking struct {
	Name  string
	Weeks []WeekSummary
}

// PreviousSaturday gets the Saturday before this date, unless the given date is a Saturday.
func PreviousSaturday(d time.Time) time.Time {
	daysToGoBack := int(d.Weekday()) + 1
	if d.Weekday() == time.Saturday {
		daysToGoBack = 0
	}
	return d.Add(time.Duration(-daysToGoBack) * 24 * time.Hour).Truncate(24 * time.Hour)
}

// ComputeWeeklySummaries summarises the input activities into the weekly marathon tracking stats.
// Output will be in chronological order.
func ComputeWeeklySummaries(activities []*strava.ActivitySummary) []WeekSummary {
	if len(activities) == 0 {
		return nil
	}
	acts := make([]*strava.ActivitySummary, len(activities))
	copy(acts, activities)
	sort.Slice(acts, func(i, j int) bool {
		return acts[i].StartDate.Before(acts[j].StartDate)
	})
	firstRunDate := acts[0].StartDate
	curWeekStart := PreviousSaturday(firstRunDate)

	var weeks [][]*strava.ActivitySummary
	var weekActs []*strava.ActivitySummary

	for _, a := range acts {
		if a.Type != strava.ActivityTypes.Run {
			continue
		}
		weekStart := PreviousSaturday(a.StartDate)
		if curWeekStart == weekStart {
			weekActs = append(weekActs, a)
		} else {
			weeks = append(weeks, weekActs)
			weekActs = []*strava.ActivitySummary{a}
			curWeekStart = weekStart
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
			Date:     PreviousSaturday(w[0].StartDate),
			Count:    len(w),
			Distance: dist,
			Time:     time.Duration(elapsed) * time.Second,
		}
		sums = append(sums, sum)
	}
	return sums
}

func FetchMarathonTrackingForUser(ctx context.Context, s *strava.Client) ([]WeekSummary, error) {
	as, err := strava.NewCurrentAthleteService(s).ListActivities().Do()
	if err != nil {
		return nil, err
	}
	return ComputeWeeklySummaries(as), nil
}

// GetUsers fetches all users from the datastore.
func GetUsers(ctx context.Context, ds *datastore.Client) ([]User, error) {
	result := make([]User, 0)
	_, err := ds.GetAll(ctx, datastore.NewQuery("User"), &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Fetches activities.
type ActivityFetcher interface {
	FetchActivities(token string) ([]*strava.ActivitySummary, error)
}

type fetcher struct {
	httpClient *http.Client
}

func (f fetcher) FetchActivities(token string) ([]*strava.ActivitySummary, error) {
	s := strava.NewClient(token, f.httpClient)
	startDate := Must(time.Parse("2006-01-02", "2018-01-01"))
	return strava.NewCurrentAthleteService(s).ListActivities().After(int(startDate.Unix())).Do()
}

// Must returns its input time or panics if there's an error.
func Must(t time.Time, err error) time.Time {
	if err != nil {
		panic(err)
	}
	return t
}

type MainTplArgs struct {
	Umt         []*UserMarathonTracking
	ClientId    string
	RedirectUri string
}

var mainTpl = template.Must(template.New("").Funcs(template.FuncMap{
	"makeTable": makeTable,
}).ParseFiles("main.html.tpl"))

// FetchUserHistory fetches each user's marathon training history.
func FetchUserHistory(users []User, fetcher ActivityFetcher) ([]*UserMarathonTracking, error) {
	results := make(chan *UserMarathonTracking)
	defer func() {
		close(results)
	}()
	for _, u := range users {
		go func(u User) {
			var result *UserMarathonTracking
			defer func() {
				results <- result
			}()
			acts, err := fetcher.FetchActivities(u.StravaToken)
			if err != nil {
				log.Printf("Failed to fetch activites: %s", err)
				fmt.Printf("Failed to fetch activites: %s", err)
				return
			}
			mt := ComputeWeeklySummaries(acts)
			name := acts[0].Athlete.FirstName

			result = &UserMarathonTracking{
				Name:  name,
				Weeks: mt,
			}
		}(u)
	}

	var err error
	var mts []*UserMarathonTracking
	for i := 0; i < len(users); i++ {
		result := <-results
		if result == nil {
			err = errors.New("couldn't fetch marathon tracking data")
		}
		mts = append(mts, result)
	}

	if err != nil {
		return nil, err
	}
	return mts, nil
}

// userKey is a key based on strava's athlete id.
func userKey(id int64) *datastore.Key {
	return datastore.IDKey("User", id, nil)
}

func makeTable(umt *UserMarathonTracking) string {
	buf := bytes.NewBuffer(nil)
	tw := tablewriter.NewWriter(buf)
	tw.SetHeader([]string{"Date", "Count", "Distance", "Duration"})
	for _, w := range umt.Weeks {

		tw.Append([]string{
			w.Date.Format("2006/01/02"),
			fmt.Sprintf("%d", w.Count),
			fmt.Sprintf("%0.1fkm", w.Distance),
			fmt.Sprintf("%s", w.Time),
		})
	}
	tw.Render()
	return buf.String()
}

func init() {
	stravaClientId, err := strconv.Atoi(os.Getenv("STRAVA_CLIENT_ID"))
	if err != nil {
		// panic(errors.New("STRAVA_CLIENT_ID not set"))
	}
	strava.ClientId = stravaClientId
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

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ctx := appengine.NewContext(r)
		ds := NewDatastore(ctx)
		users, err := GetUsers(ctx, ds)
		if err != nil {
			panic(err)
		}
		umt, err := FetchUserHistory(users, fetcher{urlfetch.Client(ctx)})
		if err != nil {
			panic(err)
		}

		err = mainTpl.ExecuteTemplate(w, "main.html.tpl", MainTplArgs{
			Umt:         umt,
			ClientId:    fmt.Sprintf("%d", stravaClientId),
			RedirectUri: "foo",
		})
		if err != nil {
			w.Write([]byte("error: " + err.Error()))
			// panic(err)
		}
	})
}
