# DCV Inspector - tool for inspecting certificate authority domain validation

DCV Inspector helps you inspect the DNS, HTTP, and SMTP requests made by a certificate authority during domain validation. You can use it to detect violations of domain validation rules, such as the use of Delegated Third Parties.

DCV Inspector creates a unique subdomain for each test. All DNS, HTTP, and SMTP requests to this subdomain (including descendants of the subdomain) are recorded and presented to you for inspection.

To use DCV inspector, visit the official instance at https://dcv-inspector.com

## Hosting DCV Inspector Yourself

You need a domain (or a subdomain of an existing domain) and a server with ports 25, 53, 80, and 443 open.

### DNS Config

Here are the DNS records for the official `dcv-inspector.com` instance:

```
dcv-inspector.com.      A    97.107.134.176
dcv-inspector.com.      AAAA 2600:3c03:e000:e8::
test.dcv-inspector.com. NS   dcv-inspector.com.
```

Replace `dcv-inspector.com` with your domain name, and the IP addresses with the IP addresses of your server.

### Installation

```
go install software.sslmate.com/src/dcv-inspector@latest
```

### Running

Run the following command, replacing the path to the database file (which will be created if needed) and the domain name:

```
dcv-inspector -db /path/to/db -domain dcv-inspector.com -smtp-listen tcp:25 -dns-listen tcp:53 -dns-udp udp:53 -http-listen tcp:80 -https-listen tcp:443
```
