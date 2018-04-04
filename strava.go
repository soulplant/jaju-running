package handlers

import (
	"context"
	"net/http"
	"time"

	strava "github.com/strava/go.strava"
	"google.golang.org/appengine/urlfetch"
)

// NewStravaClient creates a strava client using the given access token.
func NewStravaClient(ctx context.Context, accessToken string) *strava.Client {
	return strava.NewClient(accessToken, urlfetch.Client(ctx))
}

type stravaFetcher struct {
	httpClient *http.Client
}

func (f stravaFetcher) FetchActivities(token string) ([]*strava.ActivitySummary, error) {
	s := strava.NewClient(token, f.httpClient)
	return strava.NewCurrentAthleteService(s).ListActivities().After(int(time.Now().Add(-30 * 24 * time.Hour).Unix())).Do()
}
