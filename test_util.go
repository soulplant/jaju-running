package handlers

import (
	"bytes"
	"net/http"
	"os"
	"testing"

	context "golang.org/x/net/context"

	"cloud.google.com/go/datastore"
)

const projectID = "dev"

// InitTestDatastore clears out a 'well-known' datastore emulator running on localhost
// and sets environment variables that cause the client to connect to it rather than the
// real instance provided by Google.
func InitTestDatastore(t *testing.T) *datastore.Client {
	if _, err := http.Post("http://localhost:8081/reset", "", bytes.NewBufferString("")); err != nil {
		t.Fatal(err)
	}

	os.Setenv("DATASTORE_DATASET", "dev")
	os.Setenv("DATASTORE_HOST", "localhost:8081")
	os.Setenv("DATASTORE_PROJECT_ID", projectID)
	os.Setenv("DATASTORE_EMULATOR_HOST", "localhost:8081")
	os.Setenv("DATASTORE_EMULATOR_HOST_PATH", "localhost:8081/datastore")

	c, err := datastore.NewClient(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
