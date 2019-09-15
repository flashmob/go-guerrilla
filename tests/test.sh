#!/bin/sh

echo "open 127.0.0.1 2526"
sleep 1
echo "EHLO mail.test.com"
echo "MAIL FROM:<test@test.grr.la>"
echo "RCPT TO:<test@grr.la>"
echo "DATA"
echo "m"
echo "."
sleep 2