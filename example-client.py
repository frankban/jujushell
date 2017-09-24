#!/home/frankban/code/pip-venv/bin/python

import json

import websocket


def main():
    conn = websocket.create_connection('ws://localhost:8047/ws/')
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
    main()
