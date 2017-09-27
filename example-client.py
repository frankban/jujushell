#!/usr/bin/env python3

# An example WebSocket client that can be used while developing jujushell.
# Requires "apt install python3-websocket".

import json
import ssl
import sys

import websocket


SSLOPT = {'cert_reqs': ssl.CERT_NONE}


def main(address):
    url = address + '/ws/'
    print('connecting to ' + url)
    conn = websocket.create_connection(url, sslopt=SSLOPT)
    client = Client(conn)
    client.send({'operation': 'login', 'username': 'admin', 'password': 'aaa'})
    client.send({'operation': 'start'})


class Client:
    """A simple WebSocket client."""

    def __init__(self, conn):
        self.conn = conn

    def send(self, data):
        """Send the given request, wait and return a response."""
        req = json.dumps(data)

        # Send the request.
        print('--> {}'.format(req))
        try:
            self.conn.send(req)
        except Exception as err:
            print('--> ERROR: {}'.format(err))
            return

        # Wait for the response.
        try:
            resp = self.conn.recv()
        except Exception as err:
            print('<-- ERROR: {}'.format(err))
            return
        print('<-- {}'.format(resp))
        return json.loads(resp)


if __name__ == '__main__':
    address = 'wss://localhost:8047'
    if len(sys.argv) > 1:
        address = sys.argv[1]
    main(address)
