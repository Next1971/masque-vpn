package main

import "net/http"

// coverPage is what TCP 443 returns to a browser, a probe, or a client
// certificate that is not allowed to open a tunnel.
const coverPage = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Notes</title>
</head>
<body>
<h1>Notes</h1>
<p>A few lines about the weather and the street outside.</p>
</body>
</html>
`

func serveCover(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(coverPage))
}
