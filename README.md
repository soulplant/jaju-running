Strava Design Doc

Overview
Currently the Strava app pulls athlete data from Strava on demand and processes it in Go. A more flexible architecture is to have our app periodically scrape the Strava data and store it in a database which can be queried with SQL.

Drawbacks with proposed architecture
- If data is scraped periodically, it will not be fresh when a user visits the page.
- If we store the data in SQL then we can't host it on GAE because there is no free tier SQL solution.

To solve the first problem we can perform a scrape daily, as well as on page load. This will mean the latency of the page remains high, but that is not a pain point.

The second problem is more pernicious. This would have been a good exercise in SQL / GORM (albeit quite a trivial one), but we could just store the data in Datastore. The problem with that of course is that Datastore has no analytical functions in it.

Storing the data in Datastore would still be good though, as it would mean that we don't have to worry about losing old Strava data.


Channel learnings
- Closing a channel causes readers to receive a 0 value
- A closed channel can be read many times and it will synchronously return 0 values
- By default a channel is not buffered, i.e. writes block until the channel is read
- `range ch` returns values from channel until the channel is closed
- Writing to a closed channel causes a panic.

- To try: write, ok on a channel, on an unbuffered channel I expect that to block until the channel is closed
  - To this: there's no write, ok. But assuming I meant read, ok then this is correct. The ', ok' just says whether the read succeeded or is a nil value because the channel is closed.
