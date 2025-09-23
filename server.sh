#!/bin/bash
mkdir -p $HOME/server/
cd $HOME/server
if [-f "./server.jar"]; then
	echo "execute successfully"
	java -Xmx2G -jar ./server.jar nogui
else
	echo "please make sure you have your server.jar in the folder"
fi 
