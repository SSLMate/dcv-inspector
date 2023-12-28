CREATE TABLE test (
	test_id		BLOB NOT NULL PRIMARY KEY,
	started_at	DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	stopped_at	DATETIME
);
CREATE TABLE http_request (
	http_request_id	INTEGER PRIMARY KEY,
	test_id		BLOB NOT NULL REFERENCES test ON DELETE CASCADE,
	received_at	DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	remote_ip	TEXT NOT NULL,
	remote_port	INTEGER NOT NULL,
	host		TEXT NOT NULL,
	method		TEXT NOT NULL,
	url		TEXT NOT NULL,
	proto		TEXT NOT NULL,
	header_json	TEXT NOT NULL,
	https		BOOLEAN NOT NULL
);
CREATE TABLE dns_request (
	dns_request_id	INTEGER PRIMARY KEY,
	test_id		BLOB NOT NULL REFERENCES test ON DELETE CASCADE,
	received_at	DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	remote_ip	TEXT NOT NULL,
	remote_port	INTEGER NOT NULL,
	fqdn		TEXT NOT NULL,
	qtype		INTEGER NOT NULL,
	bytes		BLOB NOT NULL
);
CREATE TABLE smtp_request (
	smtp_request_id	INTEGER PRIMARY KEY,
	test_id		BLOB NOT NULL REFERENCES test ON DELETE CASCADE,
	received_at	DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	remote_ip	TEXT NOT NULL,
	remote_port	INTEGER NOT NULL,
	helo		TEXT NOT NULL,
	mail_from	TEXT NOT NULL,
	rcpt_to_json	TEXT NOT NULL,
	data		BLOB NOT NULL,
	starttls	BOOLEAN NOT NULL
);
