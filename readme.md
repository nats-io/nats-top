# nats-top

[![MIT License](http://img.shields.io/badge/license-MIT-blue.svg?style=flat-square)](https://github.com/nats-io/nats-top/blob/main/LICENSE)[![Build Status](https://travis-ci.org/nats-io/nats-top.svg?branch=main)](http://travis-ci.org/nats-io/nats-top)[![GitHub release](http://img.shields.io/github/release/nats-io/nats-top.svg?style=flat-square)](https://github.com/nats-io/nats-top/releases)

`nats-top` is a `top`-like tool for monitoring NATS servers.

```sh
$ nats-top

NATS server version 0.7.3 (uptime: 3m34s)
Server:
  Load: CPU:  58.3%  Memory: 8.6M  Slow Consumers: 0
  In:   Msgs: 568.7K  Bytes: 1.7M  Msgs/Sec: 13129.0  Bytes/Sec: 38.5K
  Out:  Msgs: 1.6M  Bytes: 4.7M  Msgs/Sec: 131290.9  Bytes/Sec: 384.6K    

Connections: 10
  HOST                 CID    NAME        SUBS    PENDING     MSGS_TO   MSGS_FROM   BYTES_TO    BYTES_FROM  LANG     VERSION  UPTIME   LAST ACTIVITY
  127.0.0.1:57487      13     example     1       12.0K       161.6K    0           484.7K      0           go       1.1.7    17s      2016-02-09 00:13:24.753062715 -0800 PST
  127.0.0.1:57488      14     example     1       11.9K       161.6K    0           484.7K      0           go       1.1.7    17s      2016-02-09 00:13:24.753040168 -0800 PST
  127.0.0.1:57489      15     example     1       12.1K       161.6K    0           484.7K      0           go       1.1.7    17s      2016-02-09 00:13:24.753069442 -0800 PST
  127.0.0.1:57490      16     example     1       12.0K       161.6K    0           484.7K      0           go       1.1.7    17s      2016-02-09 00:13:24.753057413 -0800 PST
  127.0.0.1:57491      17     example     1       12.1K       161.6K    0           484.7K      0           go       1.1.7    17s      2016-02-09 00:13:24.75307264 -0800 PST 
  127.0.0.1:57492      18     example     1       12.1K       161.6K    0           484.7K      0           go       1.1.7    17s      2016-02-09 00:13:24.753066213 -0800 PST
  127.0.0.1:57493      19     example     1       12.0K       161.6K    0           484.7K      0           go       1.1.7    17s      2016-02-09 00:13:24.753075802 -0800 PST
  127.0.0.1:57494      20     example     1       12.2K       161.6K    0           484.7K      0           go       1.1.7    17s      2016-02-09 00:13:24.753052178 -0800 PST
  127.0.0.1:57495      21     example     1       12.1K       161.6K    0           484.7K      0           go       1.1.7    17s      2016-02-09 00:13:24.753048615 -0800 PST
  127.0.0.1:57496      22     example     1       12.0K       161.6K    0           484.7K      0           go       1.1.7    17s      2016-02-09 00:13:24.753016783 -0800 PST
```

## Install

Can be installed via `go install`:

```sh
go install github.com/nats-io/nats-top@latest
```

and releases of the binary are also [available](https://github.com/nats-io/nats-top/releases)

## Usage

```
usage: nats-top [-s server] [-m http_port] [-ms https_port] [-n num_connections] [-d delay_secs] [-r max] [-o FILE] [-l DELIMITER] [-sort by]
                [-cert FILE] [-key FILE ][-cacert FILE] [-k] [-b] [-v|--version] [-u|--display-subscriptions-column]
```

- `-m http_port`, `-ms https_port`

  Monitoring http and https ports from the NATS server.

- `-n num_connections`

  Limit the connections requested to the server (default: `1024`)

- `-d delay_in_secs`

  Screen refresh interval (default: 1 second).

- `-r max`

  Specify the maximum number of times nats-top should refresh nats-stats before exiting (default: `0` which stands for `"no limit"`).

- `-o file`

  Saves the very first nats-top snapshot to the given file and exits. If '-' is passed then the snapshot is printed to the standard output.

- `-l delimiter`

  Specifies the delimiter to use for the output file when the '-o' parameter is used. By default this option is unset which means that standard grid-like plain-text output will be used.

- `-sort by `

  Field to use for sorting the connections.

- `-cert`, `-key`, `-cacert`

  Client certificate, key and RootCA for monitoring via https.

- `-k`

  Configure to skip verification of certificate.

- `-b`

  Displays traffic in raw bytes.

- `-v|--version`

  Displays the version of nats-top.

- `-u|--display-subscriptions-column`

  Makes the subscriptions-column immediately visible upon launching nats-top.

## Commands

While in top view, it is possible to use the following commands:

- **o [option]**

  Set primary sort key to **[option]**:

  Keyname may be one of: **{cid, subs, msgs_to, msgs_from, bytes_to, bytes_from, idle, last}**

  This can be set in the command line too, e.g. `nats-top -sort bytes_to`

- **n [limit]**

  Set sample size of connections to request from the server.

  This can be set in the command line as well: `nats-top -n 1`
  Note that if used in conjunction with sort, the server would respect
  both options enabling queries like _connection with largest number of subscriptions_:
  `nats-top -n 1 -sort subs`

- **s**

  Toggle displaying connection subscriptions.

- **d**

  Toggle activating DNS address lookup for clients.

- **?**

  Show help message with options.

- **q**

  Quit nats-top.

## Demo

![nats-top](https://cloud.githubusercontent.com/assets/26195/12911060/901419e0-cec4-11e5-8384-e222a891e6bf.gif)
