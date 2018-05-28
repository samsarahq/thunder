export interface MockWebSocket {
  readyState: number;
  onclose: (ev: CloseEvent) => any;
  onerror: (ev: Event) => any;
  onmessage: (ev: MessageEvent) => any;
  onopen: (ev: Event) => any;
  close(code?: number, reason?: string): void;
  send(data: any): void;
}

export function newMockWebSocket(): MockWebSocket {
  return {
    onclose: () => undefined,
    onerror: () => undefined,
    onopen: () => undefined,
    onmessage: () => undefined,
    readyState: WebSocket.CLOSED,
    close: () => undefined,
    send: () => undefined,
  };
}

export function addPings(ws: MockWebSocket) {
  ws.send = (message: string) => {
    ws.onmessage(new MessageEvent("message", { data: `{"type": "echo"}` }));
  };
}

export interface MockEvent {
  open?: true;
  message?: any;
  close?: string;
}

// newMockListener constructs a pingingwebsocket.SocketListener that logs all
// calls it receives to an internal queue, and provides a calls callback that
// pulls several items from the queue and confirms they match the expected
// callbacks.
export function newMockListener() {
  let done = false;
  const calls: MockEvent[] = [];
  let waiter: { expected: MockEvent[]; resolve: () => void } | undefined;

  const maybeFulfillWaiters = () => {
    if (waiter && waiter.expected.length <= calls.length) {
      expect(calls.slice(0, waiter.expected.length)).toEqual(waiter.expected);
      calls.splice(0, waiter.expected.length);
      waiter.resolve();
      waiter = undefined;
    }
  };

  const push = (item: MockEvent) => {
    expect(done).toEqual(false);
    calls.push(item);
    maybeFulfillWaiters();
  };

  return {
    onOpen() {
      push({ open: true });
    },
    onMessage(message: any) {
      push({ message });
    },
    onClose(reason: string) {
      push({ close: reason });
    },
    calls(expected: MockEvent[]) {
      if (waiter) {
        return Promise.reject(new Error("already have a pending waiter"));
      }
      return new Promise(resolve => {
        waiter = { expected, resolve };
        maybeFulfillWaiters();
      });
    },
    done() {
      expect(calls.length).toEqual(0);
      done = true;
    },
  };
}
