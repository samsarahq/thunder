import { Connection } from "./websocket";
import { LRU } from "./lru";
import { merge } from "./merge";

export type State = "subscribed" | "cached" | "pending" | "error";

export interface Handle {
  close(): void;
  data(): Result;
}

export interface Result {
  error?: string;
  state: State;
  valid: boolean;
  value: any;
}

interface Subscription {
  query: string;
  variables: any;
  observer: (result: Result) => void;

  value?: any;
  state: State;
  error?: string;

  retryDelay: number;
  retryHandle?: number;
}

interface Mutation {
  query: string;
  variables: any;
  resolve: (result: any) => void;
  reject: (reason: any) => void;
  timeout: number;
}

function dataFromSubscription(subscription: Subscription): Result {
  return Object.freeze({
    error: subscription.error,
    state: subscription.state,
    valid: subscription.state === "subscribed" || subscription.state === "cached",
    value: subscription.value,
  });
}

const mutationTimeout = 10000; // after 10 seconds, treat a mutation as failed

const initialRetryDelay = 1000;
const maxRetryDelay = 60000;

const cacheSize = 100;

export class Client {
  private connection: Connection;

  private nextId: number;
  private subscriptions: Map<string, Subscription>;
  private mutations: Map<string, Mutation>;

  private past: LRU<any, any>;

  constructor(url: string) {
    this.connection = new Connection(url, {
      onClose: this.onClose.bind(this),
      onMessage: this.onMessage.bind(this),
      onOpen: this.onOpen.bind(this),
    });

    this.nextId = 0;
    this.subscriptions = new Map();
    this.mutations = new Map();

    this.past = new LRU(cacheSize)
  }

  subscribe({query, variables, observer}: { query: string, variables: any, observer: (result: Result) => void }): Handle {
    const id = this.makeId();

    const cached = this.past.find({ query, variables });

    const subscription: Subscription = {
      query,
      variables,
      observer,

      value: cached,
      state: cached ? "cached" : "pending",
      error: undefined,

      retryDelay: initialRetryDelay,
      retryHandle: undefined,
    };

    this.subscriptions.set(id, subscription);

    if (this.connection.state === "connected") {
      this.connection.send({ id, type: "subscribe", message: { query, variables } });
    }

    return {
      close: () => {
        if (!this.subscriptions.get(id)) {
          return;
        }

        if (subscription.value !== undefined) {
          this.past.add({ query, variables }, subscription.value);
        }

        this.subscriptions.delete(id);
        if (this.connection.state === "connected") {
          this.connection.send({ id, type: "unsubscribe" });
        }
      },

      data: () => {
        return dataFromSubscription(subscription);
      },
    };
  }

  mutate({ query, variables }: { query: string, variables: any }) {
    if (this.connection.state !== "connected") {
      return Promise.reject(new Error("not connected"));
    }

    return new Promise<any>((resolve, reject) => {
      const id = this.makeId();
      this.connection.send({ id, type: "mutate", message: { query, variables } });

      let timeout = setTimeout(() => reject(new Error("mutation timed out")), mutationTimeout);

      const mutation: Mutation = {
        query,
        variables,
        resolve,
        reject,
        timeout,
      };

      this.mutations.set(id, mutation);
    });
  }

  private makeId() {
    return (this.nextId++).toString();
  }

  private notify(subscription: Subscription) {
    subscription.observer(dataFromSubscription(subscription));
  }

  private retry(id: string) {
    const subscription = this.subscriptions.get(id);
    if (subscription === undefined) {
      return;
    }

    this.connection.send({ id, type: "subscribe", message: { query: subscription.query, variables: subscription.variables } });
  }

  private onOpen() {
    for (const [id, subscription] of this.subscriptions.entries()) {
      this.connection.send({ id, type: "subscribe", message: { query: subscription.query, variables: subscription.variables } });
      if (subscription.retryHandle) {
        clearTimeout(subscription.retryHandle);
        subscription.retryHandle = undefined;
      }
    }
  }

  private onClose() {
    for (const [_, subscription] of this.subscriptions) {
      if (subscription.state === "subscribed") {
        subscription.state = "cached";
        this.notify(subscription);
      }
    }

    for (const mutation of this.mutations.values()) {
      mutation.reject(new Error("connection closed"));
    }
    this.mutations.clear();
  }

  private onMessage(envelope: any) {
    if (envelope.type === "update") {
      let subscription = this.subscriptions.get(envelope.id);
      if (subscription !== undefined) {
        if (subscription.state !== "subscribed") {
          subscription.state = "subscribed";
          subscription.error = undefined;
          subscription.retryDelay = initialRetryDelay;
        }

        subscription.value = merge(subscription.value, envelope.message);
        this.notify(subscription);
      }
    }

    if (envelope.type === "result") {
      let mutation = this.mutations.get(envelope.id);
      if (mutation !== undefined) {
        mutation.resolve(merge(null, envelope.message));
        clearTimeout(mutation.timeout);
        this.mutations.delete(envelope.id);
      }
    }

    if (envelope.type === "error") {
      let subscription = this.subscriptions.get(envelope.id);
      if (subscription !== undefined) {
        console.error("Subscription failed. Query:\n",
          subscription.query,
          "\nVariables:\n",
          subscription.variables,
          "\nError:\n",
          envelope.message);

        subscription.state = "error";
        subscription.error = envelope.message;

        subscription.retryHandle = setTimeout(
          () => this.retry(envelope.id), subscription.retryDelay);
        subscription.retryDelay = Math.min(maxRetryDelay, subscription.retryDelay * 2);
        this.notify(subscription);
      }

      let mutation = this.mutations.get(envelope.id);
      if (mutation !== undefined) {
        console.error("Mutation failed. Query:\n",
          mutation.query,
          "\nVariables:\n",
          mutation.variables,
          "\nError:\n",
          envelope.message);

        mutation.reject(new Error(envelope.message));
        clearTimeout(mutation.timeout);
        this.mutations.delete(envelope.id);
      }
    }
  }
}
