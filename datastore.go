package handlers

import (
	"context"

	strava "github.com/strava/go.strava"
	"google.golang.org/appengine/datastore"
)

// userKey is a key based on strava's athlete id.
func userKey(ctx context.Context, id int64) *datastore.Key {
	return datastore.NewKey(ctx, "User", "", id, nil)
}

// RegisterNewUser saves a new user's Strava token and basic details in the datastore.
func RegisterNewUser(ctx context.Context, auth *strava.AuthorizationResponse) (*User, error) {
	user := User{
		FirstName:   auth.Athlete.FirstName,
		LastName:    auth.Athlete.LastName,
		StravaToken: auth.AccessToken,
	}
	_, err := datastore.Put(ctx, userKey(ctx, auth.Athlete.Id), &user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUsers fetches all users from the datastore.
func GetUsers(ctx context.Context) ([]User, error) {
	result := make([]User, 0)
	_, err := datastore.NewQuery("User").Order("FirstName").GetAll(ctx, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
