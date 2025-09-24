#!/bin/bash
echo "are you going to install in this dictionary? (yes/no)"
read t
if [ $t == "no" ]; then
	exit
fi

curl -L -O "https://github.com/galoistom/web_for_server/blob/main/mainpack.tar.gz"
tar -xzvf ./mainpack.tar.gz
cd ./mainpack/
echo "choose your operating system (1 for windows, 2 for mac, 3 for linux)"
read oper
if [ $oper == "1" ]; then
	curl -L -O "https://github.com/galoistom/web_for_server/blob/main/build/web_for_server_win_x86.exe"
elif [ $oper == "2" ]; then
	curl -L -O "https://github.com/galoistom/web_for_server/blob/main/build/web_for_server_mac_arm"
elif [ $oper == "3" ]; then
	echo "choose your chip (1 for x86, 2 for arm)"
	read chip
	if [ $chip == "1" ]; then
		curl -L -O "https://github.com/galoistom/web_for_server/blob/main/build/web_for_server_linux_x86"
	elif [ $chip == "2" ]; then
		curl -L -O "https://github.com/galoistom/web_for_server/blob/main/build/web_for_server_linux_x86"
	else
		echo "input wrongly"
		exit
	fi
else
	echo "input wrongly"
fi
