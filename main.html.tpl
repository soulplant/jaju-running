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

  <a href="https://www.strava.com/oauth/authorize?client_id={{.ClientId}}&redirect_uri={{.RedirectUri | urlquery}}&response_type=code">
    Register
  </a>
</div>
