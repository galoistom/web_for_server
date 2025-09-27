#!/bin/bash
read POST
if [ -d $POST ]; then
	cd $POST
	if [ -f "./server.jar" ]; then
		echo "execute successfully"
		java -Xmx7G -jar ./server.jar nogui
	else
		echo "please make sure you have your server.jar in the folder"
	fi 
else
	echo "make sure you are in the write folder"
fi
