# http-proxy

An HTTP server that proxies all requests to other HTTP servers on localhost,
based on the incoming request's Host header.

## Installation

```
go get github.com/peterbourgon/http-proxy
```

## Usage

```
USAGE
  http-proxy [flags]

FLAGS
  -cert server.crt    TLS certificate
  -config proxy.conf  config file
  -example false      print example config file to stdout and exit
  -http :80           serve HTTP on this address (optional)
  -key server.key     TLS key
  -tls ...            serve TLS on this address (optional)
```

proxy.conf

```
example.com, www.example.com: 8081
subdomain.example.com: 8082
www.website.online: 9091
```

