#!/bin/sh
# Proxy settings for Claude CLI API access
export HTTP_PROXY=http://127.0.0.1:8081/
export HTTPS_PROXY=http://127.0.0.1:8081/
export http_proxy=http://127.0.0.1:8081/
export https_proxy=http://127.0.0.1:8081/
export ALL_PROXY=socks://127.0.0.1:1081/
export all_proxy=socks://127.0.0.1:1081/
export NO_PROXY=localhost,127.0.0.1/8,::1,192.168.31.0/24
export no_proxy=localhost,127.0.0.1/8,::1,192.168.31.0/24

curl ipinfo.io | jq