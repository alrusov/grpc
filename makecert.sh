#!/bin/bash

set -e

openssl req -x509 -nodes -days 3650 -newkey rsa:2048 -keyout server.key -out server.crt -subj "/C=RU/ST=Moscow/L=Moscow/O=Company/OU=Department/CN=127.0.0.1"
cat server.key server.crt >server.pem
rm server.key server.crt
