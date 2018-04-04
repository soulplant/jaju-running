package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/strava/go.strava"

	"context"

	"google.golang.org/appengine"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/urlfetch"
)

// User represents a Strava user who has authorised access to their data.
type User struct {
	FirstName   string
	LastName    string
	StravaToken string
}

// WeekSummary summarises runs that occur in the same week.
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

// UserMarathonTracking is a history of weekly marathon training stats for a given user.
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
	var runs []*strava.ActivitySummary
	for _, act := range activities {
		if act.Type == strava.ActivityTypes.Run {
			runs = append(runs, act)
		}
	}
	if len(runs) == 0 {
		return nil
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].StartDate.Before(runs[j].StartDate)
	})
	firstRunDate := runs[0].StartDate
	curWeekStart := PreviousSaturday(firstRunDate)

	var allWeeks [][]*strava.ActivitySummary
	var thisWeek []*strava.ActivitySummary

	for _, run := range runs {
		weekStart := PreviousSaturday(run.StartDate)
		if curWeekStart == weekStart {
			thisWeek = append(thisWeek, run)
		} else {
			allWeeks = append(allWeeks, thisWeek)
			thisWeek = []*strava.ActivitySummary{run}
			curWeekStart = weekStart
		}
	}
	if thisWeek != nil {
		allWeeks = append(allWeeks, thisWeek)
	}
	var result []WeekSummary
	for _, w := range allWeeks {
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
		result = append(result, sum)
	}
	return result
}

// ActivityFetcher fetches activities from Strava for a given user.
type ActivityFetcher interface {
	FetchActivities(token string) ([]*strava.ActivitySummary, error)
}

func DoAsync(f func(interface{}) (interface{}, error), inputs []interface{}) ([]interface{}, error) {
	var wg sync.WaitGroup
	wg.Add(len(inputs))
	result := make([]interface{}, len(inputs))
	fail := make(chan error, len(inputs))
	for i, in := range inputs {
		go func(i int, in interface{}) {
			defer wg.Done()
			out, err := f(in)
			if err != nil {
				fail <- err
				return
			}
			result[i] = out
		}(i, in)
	}
	wg.Wait()
	close(fail)
	err := <-fail
	if err != nil {
		return nil, err
	}
	return result, nil
}

func FetchUsersActivity(users []User, fetcher ActivityFetcher) ([][]*strava.ActivitySummary, error) {
	var cs []chan []*strava.ActivitySummary
	for _, u := range users {
		// Note, we give this capacity 1 so that we don't leak goroutines. When
		// iterating over these channels later we don't guarantee that we read
		// them all, so having capacity in the channel means that the goroutine
		// can terminate and the channel itself can get gc'd.
		// An alternative would be to read all the channels afterwards which
		// would have the effect of unblocking the goroutines waiting for a
		// chance to write.
		c := make(chan []*strava.ActivitySummary, 1)
		cs = append(cs, c)
		go func(u User) {
			defer close(c)
			acts, err := fetcher.FetchActivities(u.StravaToken)
			if err != nil {
				return
			}
			c <- acts
		}(u)
	}
	var result [][]*strava.ActivitySummary
	for _, c := range cs {
		val, ok := <-c
		if !ok {
			return nil, errors.New("Failed")
		}
		result = append(result, val)
	}
	return result, nil
}

func FetchUsersActivity3(users []User, fetcher ActivityFetcher) ([][]*strava.ActivitySummary, error) {
	var wg sync.WaitGroup
	wg.Add(len(users))
	fail := make(chan error, len(users))
	var m sync.Map
	for i, u := range users {
		go func(i int, u User) {
			defer wg.Done()
			act, err := fetcher.FetchActivities(u.StravaToken)
			if err != nil {
				fail <- err
				return
			}
			m.Store(i, act)
		}(i, u)
	}
	wg.Wait()
	close(fail)
	err := <-fail
	if err != nil {
		return nil, err
	}
	r := make([][]*strava.ActivitySummary, len(users))
	m.Range(func(key, value interface{}) bool {
		r[key.(int)] = value.([]*strava.ActivitySummary)
		return true
	})
	return r, nil
}

func FetchUsersActivity2(users []User, fetcher ActivityFetcher) ([][]*strava.ActivitySummary, error) {
	var wg sync.WaitGroup
	wg.Add(len(users))

	type resp struct {
		i    int
		acts []*strava.ActivitySummary
	}
	fail := make(chan error, len(users))
	rchan := make(chan resp, len(users))

	for i, u := range users {
		go func(i int, u User) {
			defer wg.Done()
			act, err := fetcher.FetchActivities(u.StravaToken)
			if err != nil {
				fail <- err
				return
			}
			rchan <- resp{i, act}
		}(i, u)
	}
	wg.Wait()
	close(rchan)
	select {
	case err := <-fail:
		return nil, err
	default:
	}
	result := make([][]*strava.ActivitySummary, len(users))
	for r := range rchan {
		result[r.i] = r.acts
	}
	return result, nil
}

func FetchUserHistory(ctx context.Context, users []User, fetcher ActivityFetcher) ([]*UserMarathonTracking, error) {
	acts, err := FetchUsersActivity(users, fetcher)
	if err != nil {
		return nil, err
	}
	var result []*UserMarathonTracking
	for i, act := range acts {
		umt := &UserMarathonTracking{
			Name:  users[i].FirstName,
			Weeks: ComputeWeeklySummaries(act),
		}
		result = append(result, umt)
	}
	return result, nil
}

// FetchUserHistory fetches each user's marathon training history.
func FetchUserHistory2(ctx context.Context, users []User, fetcher ActivityFetcher) ([]*UserMarathonTracking, error) {
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
				log.Errorf(ctx, "Failed to fetch activites: %s", err.Error())
				return
			}
			mt := ComputeWeeklySummaries(acts)
			name := u.FirstName

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

func init() {
	stravaClientID, err := strconv.Atoi(os.Getenv("STRAVA_CLIENT_ID"))
	if err != nil {
		fmt.Printf("STRAVA_CLIENT_ID not set\n")
		return
	}
	strava.ClientId = stravaClientID
	stravaClientSecret := os.Getenv("STRAVA_CLIENT_SECRET")
	if stravaClientSecret == "" {
		panic(errors.New("STRAVA_CLIENT_SECRET not set"))
	}
	strava.ClientSecret = stravaClientSecret

	auth := strava.OAuthAuthenticator{
		CallbackURL: "/oauth_callback",
		RequestClientGenerator: func(r *http.Request) *http.Client {
			return urlfetch.Client(appengine.NewContext(r))
		},
	}
	http.HandleFunc(auth.CallbackURL, auth.HandlerFunc(func(auth *strava.AuthorizationResponse, w http.ResponseWriter, r *http.Request) {
		ctx := appengine.NewContext(r)
		_, err := RegisterNewUser(ctx, auth)
		if err != nil {
			handleError(w, err)
			return
		}
		http.Redirect(w, r, "/", http.StatusFound)
	}, func(err error, w http.ResponseWriter, r *http.Request) {
		handleError(w, err)
	}))

	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok")) // nolint: errcheck
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ctx := appengine.NewContext(r)
		users, err := GetUsers(ctx)
		if err != nil {
			handleError(w, err)
			return
		}
		umt, err := FetchUserHistory(ctx, users, stravaFetcher{urlfetch.Client(ctx)})
		if err != nil {
			handleError(w, err)
			return
		}
		err = mainTpl.Execute(w, mainTplArgs{
			Umt:         umt,
			ClientID:    fmt.Sprintf("%d", stravaClientID),
			RedirectURI: "https://jaju-running.appspot.com/oauth_callback",
		})
		if err != nil {
			handleError(w, err)
			return
		}
	})
}

func handleError(w http.ResponseWriter, err error) {
	w.WriteHeader(500)
	w.Write([]byte(fmt.Sprintf("Failed: %s", err))) // nolint: errcheck
}
