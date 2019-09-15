#!/bin/sh
#echo -e "EHLO mail.test.com\r\nMAIL FROM:test@test.grr.la\r\nMAIL TO:test@grr.la\r\nDATA\r\nthis is a test\r\n.\r\n" | openssl s_client -starttls smtp -crlf -connect 127.0.0.1:2526 -ign_eof


echo "open 127.0.0.1 2526"
sleep 1
echo "EHLO mail.test.com"
echo "MAIL FROM:<test@test.grr.la>"
echo "RCPT TO:<flashmob@grr.la>"
echo "RCPT TO:<test@grr.la>"
#echo "RCPT TO:<guerrilla@grr.la>"
echo "DATA"
echo "To: test@sharklasers.com"
echo "Subject: Tester 123"
echo "From: <=?utf-8?Q?=42=45=47=49=4E=20=2F=20=28=7C=29=7C=3C=7C=3E=7C=40=7C=2C=7C=3B=7C=3A=7C=5C=7C=22=7C=2F=7C=5B=7C=5D=7C=3F=7C=2E=7C=3D=20=2F=20=00=20=50=41=53=53=45=44=20=4E=55=4C=4C=20=42=59=54=45=20=2F=20=0D=0A=20=50=41=53=53=45=44=20=43=52=4C=46=20=2F=20=45=4E=44?=@companyemail.com>"
echo "Message-ID: <0.0.18.30.1D2A0B2789BC3CE.0@uspmta194113.emarsys.net>"
echo "Sender: \"newz\"<news@email.posterxxl.de>"
echo "Reply-to: <replys@grr.la>"
echo "\n\n"
echo "this is a test"
echo "."
sleep 1

echo "MAIL FROM:<test@test.grr.la>"
echo "RCPT TO:<flashmob@grr.la>"

echo "DATA"
echo "To: test@sharklasers.com"
echo "Subject: Test"
echo "From: \"posterXXL VIP NEWS\" <news@email.posterxxl.de>"
echo "Sender: \"newz\"<news@email.posterxxl.de>"
echo "Content-Type: text/html; charset=\"UTF-8\""
echo "Reply-to: <ohmygosh@grr.la>"
echo "\n\n"
echo "this is a test 2"
echo "."

echo "QUIT"
sleep 1
