# nats-top

[![MIT License](http://img.shields.io/badge/license-MIT-blue.svg?style=flat-square)](https://github.com/nats-io/nats-top/blob/master/LICENSE)[![Build Status](https://travis-ci.org/nats-io/nats-top.svg?branch=master)](http://travis-ci.org/nats-io/nats-top)[![GitHub release](http://img.shields.io/github/release/nats-io/nats-top.svg?style=flat-square)](https://github.com/nats-io/nats-top/releases)

`nats-top` is a `top`-like tool for monitoring gnatsd servers.

```sh
$ nats-top

gnatsd version 0.6.4 (uptime: 31m42s)
Server:
  Load: CPU: 0.8%   Memory: 5.9M  Slow Consumers: 0
  In:   Msgs: 34.2K  Bytes: 3.0M  Msgs/Sec: 37.9  Bytes/Sec: 3389.7
  Out:  Msgs: 68.3K  Bytes: 6.0M  Msgs/Sec: 75.8  Bytes/Sec: 6779.4

Connections: 4
  HOST                 CID      SUBS    PENDING     MSGS_TO     MSGS_FROM   BYTES_TO    BYTES_FROM  LANG     VERSION SUBSCRIPTIONS
  127.0.0.1:56134      2        5       0           11.6K       11.6K       1.1M        905.1K      go       1.1.0   foo, hello
  127.0.1.1:56138      3        1       0           34.2K       0           3.0M        0           go       1.1.0    _INBOX.a96f3f6853616154d23d1b5072
  127.0.0.1:56144      4        5       0           11.2K       11.1K       873.5K      1.1M        go       1.1.0   foo, hello
  127.0.0.1:56151      5        8       0           11.4K       11.5K       1014.6K     1.0M        go       1.1.0   foo, hello
```

## Install

Can be installed via `go get`:

```sh
go get github.com/nats-io/nats-top
```

## Usage

```
nats-top [-s server] [-m monitor] [-n num_connections] [-d delay_in_secs] [-sort by]
```

- `-m monitor`

  Monitoring http port from gnatsd.

- `-n num_connections`

  Limit the connections requested to the server (default: `1024`)

- `-d delay_in_secs`

  Screen refresh interval (default: 1 second).

- `-sort by `

  Field to use for sorting the connections.

## Commands

While in top view, it is possible to use the following commands:

- **o [option]**

  Set primary sort key to **[option]**:

  Keyname may be one of: **{cid, subs, msgs_to, msgs_from, bytes_to, bytes_from}**

  This can be set in the command line too, e.g. `nats-top -sort bytes_to`

- **n [limit]**

  Set sample size of connections to request from the server.

  This can be set in the command line as well: `nats-top -n 1`
  Note that if used in conjunction with sort, the server would respect
  both options enabling queries like _connection with largest number of subscriptions_:
  `nats-top -n 1 -sort subs`

- **s**

  Toggle displaying connection subscriptions.

- **?**

  Show help message with options.

- **q**

  Quit nats-top.
