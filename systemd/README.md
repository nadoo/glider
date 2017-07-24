## Service

service file:

```bash
# copy service file to systemd
cp systemd/glider@.service /etc/systemd/system/
```

config file: ***server***.conf
```bash
# copy config file to /etc/glider/
mkdir /etc/glider/
cp glider.conf.example /etc/glider/server.conf
```

enable and start service: glider@***server***
```bash
# enable and start service
systemctl enable glider@server
systemctl start glider@server
```
See [glider@.service](https://github.com/nadoo/glider/blob/master/systemd/glider%40.service)
