module software.sslmate.com/src/dcv-inspector

go 1.26.0

require (
	github.com/emersion/go-smtp v0.26.0
	github.com/kentik/patricia v1.2.2
	github.com/mattn/go-sqlite3 v1.14.24
	github.com/miekg/dns v1.1.73
	src.agwa.name/go-dbutil v0.9.0
	src.agwa.name/go-listener v0.7.2
)

require (
	github.com/emersion/go-sasl v0.0.0-20241020182733-b788ff22d5a6 // indirect
	github.com/lib/pq v1.11.2 // indirect
	golang.org/x/crypto v0.59.0 // indirect
	golang.org/x/net v0.63.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.44.0 // indirect
)

retract v0.3.1 // Fails to build from source.
