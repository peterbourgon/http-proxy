#!/usr/bin/env sh

# https://community.fastly.com/t/setting-up-self-signed-tls-in-fastly-and-my-origin/

echo "Enter a reasonable passphrase when prompted."
openssl genrsa -des3 -out server.key.secure 2048

echo "Generating key file"
openssl rsa -in server.key.secure -out server.key

echo "The CN may be important, if creating a cert for a specific site, enter its FQDN."
openssl req -new -key server.key -out server.csr

echo "Generating certificate file."
openssl x509 -req -days 365 -in server.csr -signkey server.key -out server.crt

