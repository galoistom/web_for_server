This is my first webpage **>_<**

I run a minecraft server on my raspberrypi, and my friend want to restart server from time to time, but I do not want him to gain access to the hole machine, so I build this webpage. It is also a practice as I have been learning golang for a couple days. The front end is genreated by gemini as I bearly know how to write html. **QWQ**

I have build it for my pi, and if you want to use it, you'd better rewrite the shell script in order to start your server (might be more difficult if you are on windows).

Refresh the page by clicking buttons (because I'm lazy)

https://www.bilibili.com/video/BV1fD4y1m7TD

https://www.bilibili.com/video/BV1Xv411k7Xn

These vedios are great and helps me a lot **orz**

---

# Usage
Simply Download the correct version for your os and chip, then it will automatically genreate the config file needed as config.json.

Modify the configs to fit you needs and restart, it will load all of your settings.

## Configs

rcon_host, password: the port of rcon that is connected to you minecraft server, which should meet your settings in server.porperties

port: the port you want the web to be

server_path: the position of your server, should be absolut position.

start_command: the command to start your server, it will be executed in server_path, so accept relative path and your own script

show_log: whether you want to present the log file on the web, use ture or false.

## Commandline 

```bash
web_for_server [OPTIONS] <config.json>
```
where options include -c/--config for setting config file, -d/--debug for commandline control, and -h/--help for help

once you enter the debug mode, you are able to send command directly to the minecraft server through commandline, except for "start" which is used to start the server, and "exit" is to exit the whole program.
