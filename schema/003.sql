CREATE TABLE dns_record (
	dns_record_id	INTEGER PRIMARY KEY,
	test_id		BLOB NOT NULL REFERENCES test ON DELETE CASCADE,
	subdomain	TEXT NOT NULL,
	type		INTEGER NOT NULL,
	data_json	TEXT NOT NULL
);
CREATE INDEX dns_record_index ON dns_record (test_id, subdomain);

CREATE TABLE http_file (
	http_file_id	INTEGER PRIMARY KEY,
	test_id		BLOB NOT NULL REFERENCES test ON DELETE CASCADE,
	scheme		TEXT NOT NULL,
	subdomain	TEXT NOT NULL,
	path		TEXT NOT NULL,
	content		TEXT NOT NULL
);
CREATE UNIQUE INDEX http_file_index ON http_file (test_id, scheme, subdomain, path);
