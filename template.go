package handlers

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/olekukonko/tablewriter"
)

type mainTplArgs struct {
	Umt         []*UserMarathonTracking
	ClientID    string
	RedirectURI string
}

func makeTable(umt *UserMarathonTracking) string {
	buf := bytes.NewBuffer(nil)
	tw := tablewriter.NewWriter(buf)
	tw.SetHeader([]string{"Date", "Count", "Distance", "Duration"})
	for _, w := range umt.Weeks {

		tw.Append([]string{
			w.Date.Format("2006/01/02"),
			fmt.Sprintf("%d", w.Count),
			fmt.Sprintf("%0.1fkm", w.Distance/1000),
			fmt.Sprintf("%dh %dm", int(w.Time.Hours()), int(w.Time.Minutes())%60),
		})
	}
	tw.Render()
	return buf.String()
}

const mainTplText = `
<div>
  <div style="display: flex; justify-content: center">
  {{range .Umt}}
    <div style="padding: 0 1em">
      <pre>{{.Name}}</pre>
      <pre>
{{. | makeTable}}
      </pre>
    </div>
  {{end}}
  </div>

  <a href="https://www.strava.com/oauth/authorize?client_id={{.ClientId}}&redirect_uri={{.RedirectURI | urlquery}}&response_type=code">
    Register
  </a>
</div>
`

var mainTpl = template.Must(template.New("").Funcs(template.FuncMap{
	"makeTable": makeTable,
}).Parse(mainTplText))
