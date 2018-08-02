## Service

### Install

#### 1. copy binary file

```bash
cp glider /usr/bin/
```

#### 2. add service file

```bash
# copy service file to systemd
cp systemd/glider@.service /etc/systemd/system/
```

#### 3. add config file: ***server***.conf

```bash
# copy config file to /etc/glider/
mkdir /etc/glider/
cp ./config/glider.conf.example /etc/glider/server.conf
```

#### 4. enable and start service: glider@***server***

```bash
# enable and start service
systemctl enable glider@server
systemctl start glider@server
```

See [glider@.service](glider%40.service)

### Open file limit

#### 1. setup system limit

vim /etc/security/limits.d/unlimited.conf

```bash
* soft nofile unlimited
* hard nofile unlimited
* soft nproc unlimited
* hard nproc unlimited
```

#### 2. change **systemd** limit

```bash
echo DefaultLimitNOFILE=102400 >> /etc/systemd/system.conf
echo DefaultLimitNOFILE=102400 >> /etc/systemd/user.conf
```

#### 3. reexec **systemd**

```bash
systemctl daemon-reexec
```

#### 4. restart glider service

```bash
systemctl restart glider@server
```

#### 5. check the limits of PID

```bash
cat /proc/PID/limits
```
