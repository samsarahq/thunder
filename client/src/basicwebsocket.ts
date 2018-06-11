WebSocket;
export interface BasicWebSocket {
  readonly readyState: number;

  onclose: (ev: CloseEvent) => any;
  onerror: (ev: Event) => any;
  onmessage: (ev: MessageEvent) => any;
  onopen: (ev: Event) => any;
  close(code?: number, reason?: string): void;
  send(data: any): void;
}

export const BasicWebSocket = Object.freeze({
  CONNECTING: 0,
  OPEN: 1,
  CLOSING: 2,
  CLOSED: 3,
});
