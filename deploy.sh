#!/bin/bash

moveto=/home/gmail/guerrillad-$(date +"%m-%d-%y-%H-%m-%s")
make clean
make guerrillad
status=$?

if test $status -eq 0
then
    set -e
    echo "renaming: ssh root@grr.la mv /home/gmail/guerrillad $moveto"
	ssh root@grr.la "mv /home/gmail/guerrillad $moveto"
	echo "copying"
	scp ./guerrillad root@grr.la:/home/gmail
	echo "setcap time"
	ssh root@grr.la "sudo setcap 'cap_net_bind_service=+ep' /home/gmail/guerrillad"
	echo "killing"
	ssh root@grr.la "xargs kill < /home/gmail/go-guerrilla.pid"
	# sudo -i -u gmail /home/gmail/guerrillad -c /home/gmail/goguerrilla.conf serve >> /home/gmail/smtpd_out.log 2>&1 &
	#
	echo "done"
else
	echo "build failed"
fi