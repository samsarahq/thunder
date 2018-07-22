import { LRUCache } from "./lru";
import { merge } from "./merge";
import { ConnectFunction, InEnvelope, OutEnvelope } from "./pingingwebsocket";
import { ReconnectingWebSocket } from "./reconnectingwebsocket";
import { MutationSpec } from "./spec";

export const ErrorNotConnected = "not connected";
export const ErrorMutationTimeout = "mutation timed out";
export const ErrorConnectionClosed = "connection closed";

export interface GraphQLData<Result> {
  data: GraphQLResult<Result>;
}

export type GraphQLResult<Result> =
  | {
      state: "error";
      error: GraphQLError;
      value: never;
    }
  | {
      state: "loading";
      value?: Result;
      error: never;
    }
  | {
      state: "subscribed";
      value: Result;
      error: never;
    }
  | {
      state: "cached";
      value: Result;
      error: never;
    };

function dataFromSubscription<Result, Input>(
  subscription: Subscription<Result, Input>,
) {
  return Object.freeze({
    state: subscription.state,
    value: subscription.value,
    error: subscription.error,
  }) as GraphQLResult<Result>;
}

export class GraphQLError extends Error {
  constructor(message: string) {
    super(message);

    // Set the prototype explicitly.
    Object.setPrototypeOf(this, GraphQLError.prototype);
  }
}

// n.b `valid` for graphql subscriptions means "valid for the current query,"
// i.e. "cached" and "subscribed" are both valid, but "previous" is not.
export type SubscriptionState =
  // The current data is valid for this query, but potentially stale.
  | "cached"
  // The current data is being loaded
  | "loading"
  // The current data is valid and current
  | "subscribed"
  // The subscription returned an error from the backend
  | "error";

interface GraphqlQuery<InputVariables> {
  query: string;
  variables: InputVariables;
}

export interface Subscription<Result, InputVariables>
  extends GraphqlQuery<InputVariables> {
  state: SubscriptionState;
  retryDelay: number;
  retryHandle: any;
  observer: (data: GraphQLResult<Result>) => void;
  value: Result | undefined;
  error?: GraphQLError;
}

export interface Mutation<InputVariables> extends GraphqlQuery<InputVariables> {
  resolve: any;
  reject: any;
  timeout: number;
}

export type Observer<Result> = (data: Result) => void;

class Connection {
  nextId = 0;
  subscriptions = new Map<string, Subscription<object, object>>();
  mutations = new Map<string, Mutation<object | undefined>>();
  past = new LRUCache(100);
  initialRetryDelay = 1000;
  maxRetryDelay = 60000;
  mutationTimeoutMs = 10000;
  socket: ReconnectingWebSocket;

  constructor(connectFunction: ConnectFunction) {
    this.socket = new ReconnectingWebSocket(connectFunction, {
      onOpen: this.handleOpen.bind(this),
      onClose: this.handleClose.bind(this),
      onMessage: this.handleMessage.bind(this),
    });
    this.socket.reconnect();
  }

  makeId() {
    return (this.nextId++).toString();
  }

  subscribe<QueryResult extends object, QueryInputVariables extends object>({
    query,
    variables,
    observer,
  }: {
    query: string;
    variables: QueryInputVariables;
    observer: Observer<GraphQLResult<QueryResult>>;
  }) {
    const id = this.makeId();

    const cached = this.past.find({
      query,
      variables,
    }) as QueryResult | undefined;

    const subscription: Subscription<QueryResult, QueryInputVariables> = {
      state: cached ? "cached" : "loading",
      retryDelay: this.initialRetryDelay,
      retryHandle: undefined,
      query,
      variables,
      observer,
      value: cached,
      error: undefined,
    };

    this.subscriptions.set(id, subscription as Subscription<any, any>);

    if (this.socket.state === "connected") {
      this.send({
        id,
        type: "subscribe",
        message: { query, variables },
      });
    }

    return {
      close: () => {
        const sub = this.subscriptions.get(id);
        if (!sub) {
          return;
        }

        if (sub.value !== undefined) {
          this.past.add({ query, variables }, sub.value);
        }

        this.subscriptions.delete(id);
        if (this.socket.state === "connected") {
          this.send({ id, type: "unsubscribe" });
        }
      },
      data: () => {
        return dataFromSubscription(subscription);
      },
    };
  }

  mutate<
    MutationInputVariables extends object | undefined,
    MutationOutput extends object
  >({
    query,
    variables,
  }: MutationInputVariables extends undefined
    ? {
        query: MutationSpec<MutationOutput, MutationInputVariables>;
        variables?: undefined;
      }
    : {
        query: MutationSpec<MutationOutput, MutationInputVariables>;
        variables: MutationInputVariables;
      }): Promise<MutationOutput>;

  mutate<MutationInputVariables extends object | undefined, MutationOutput>({
    query,
    variables,
  }: MutationInputVariables extends undefined
    ? {
        query: string;
        variables?: undefined;
      }
    : {
        query: string;
        variables: MutationInputVariables;
      }): Promise<MutationOutput>;

  mutate<
    MutationInputVariables extends object | undefined,
    MutationOutput extends object
  >({
    query: queryInput,
    variables,
  }: MutationInputVariables extends undefined
    ? {
        query: string | MutationSpec<MutationOutput, MutationInputVariables>;
        variables?: undefined;
      }
    : {
        query: string | MutationSpec<MutationOutput, MutationInputVariables>;
        variables: MutationInputVariables;
      }): Promise<MutationOutput> {
    const id = this.makeId();

    const query =
      typeof queryInput === "string" ? queryInput : queryInput.query;

    if (this.socket.state === "connected") {
      this.send({ id, type: "mutate", message: { query, variables } });
    } else {
      return Promise.reject(new Error(ErrorNotConnected));
    }

    return new Promise((resolve, reject) => {
      const mutation: Mutation<object | undefined> = {
        query,
        variables,
        timeout: setTimeout(
          () => mutation.reject(new Error(ErrorMutationTimeout)),
          this.mutationTimeoutMs,
        ),
        resolve,
        reject,
      };

      this.mutations.set(id, mutation);
    });
  }

  notify(subscription: Subscription<object, object>) {
    subscription.observer(dataFromSubscription(subscription));
  }

  retry(id: string) {
    const subscription = this.subscriptions.get(id);
    if (subscription === undefined) {
      return;
    }

    this.send({
      id,
      type: "subscribe",
      message: { query: subscription.query, variables: subscription.variables },
    });
  }

  open() {
    this.socket.reconnect();
  }

  close() {
    this.socket.close();
  }

  send(message: OutEnvelope) {
    this.socket.send(message);
  }

  handleOpen() {
    for (const [id, subscription] of this.subscriptions) {
      this.send({
        id,
        type: "subscribe",
        message: {
          query: subscription.query,
          variables: subscription.variables,
        },
      });
      if (subscription.retryHandle) {
        clearTimeout(subscription.retryHandle);
        subscription.retryHandle = undefined;
      }
    }
  }

  handleClose() {
    for (const [, subscription] of this.subscriptions) {
      if (subscription.state === "subscribed") {
        subscription.state = "cached";
        this.notify(subscription);
      }
    }

    for (const [, mutation] of this.mutations) {
      mutation.reject(new Error(ErrorConnectionClosed));
    }
    this.mutations.clear();
  }

  handleMessage(envelope: InEnvelope) {
    let subscription: Subscription<object, object> | undefined;
    let mutation: Mutation<object | undefined> | undefined;

    switch (envelope.type) {
      case "update":
        subscription = this.subscriptions.get(envelope.id);
        if (subscription !== undefined) {
          if (subscription.state !== "subscribed") {
            subscription.state = "subscribed";
            subscription.error = undefined;
            subscription.retryDelay = this.initialRetryDelay;
          }

          subscription.value = merge(subscription.value, envelope.message);
          this.notify(subscription);
        }
        break;

      case "result":
        mutation = this.mutations.get(envelope.id);
        if (mutation !== undefined) {
          mutation.resolve(merge(null, envelope.message));
          clearTimeout(mutation.timeout);
          this.mutations.delete(envelope.id);
        }
        break;

      case "error":
        subscription = this.subscriptions.get(envelope.id);
        if (subscription !== undefined) {
          console.error(
            "Subscription failed. Query:\n",
            subscription.query,
            "\nVariables:\n",
            subscription.variables,
            "\nError:\n",
            envelope.message,
          );

          subscription.state = "error";
          subscription.error = new GraphQLError(envelope.message);

          subscription.retryHandle = setTimeout(
            () => this.retry(envelope.id),
            subscription.retryDelay,
          );
          subscription.retryDelay = Math.min(
            this.maxRetryDelay,
            subscription.retryDelay * 2,
          );
          this.notify(subscription);
        }

        mutation = this.mutations.get(envelope.id);
        if (mutation !== undefined) {
          console.error(
            "Mutation failed. Query:\n",
            mutation.query,
            "\nVariables:\n",
            mutation.variables,
            "\nError:\n",
            envelope.message,
          );

          mutation.reject(new GraphQLError(envelope.message));
          clearTimeout(mutation.timeout);
          this.mutations.delete(envelope.id);
        }
        break;

      default:
        break;
    }
  }
}

export { Connection };
