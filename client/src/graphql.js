import { BaseConnection } from "./websocket";
import { isEqual, assign } from "lodash";

const mutationTimeout = 10000; // after 10 seconds, treat a mutation as failed

function merge(value, update) {
  if (Array.isArray(update)) {
    if (typeof update[0] === "object") {
      return Object.freeze(update[0]);
    } else {
      return update[0];
    }
  }

  if (Array.isArray(update)) {
    return Object.freeze(update.map((value) => merge(null, value)));
  }

  if (typeof update !== "object" || update === null) {
    return update;
  }

  let result;
  if (Array.isArray(value)) {
    result = [];
    for (var x of (update.$ || [[0, value.length]])) {
      if (Array.isArray(x)) {
        for (var i = x[0]; i < x[0] + x[1]; i++) {
          result.push(value[i]);
        }
      } else if (result[x] === -1) {
        result.push(undefined);
      } else {
        result.push(value[x]);
      }
    }
    delete(update.$);

    for (const key of Object.keys(update)) {
      result[key] = merge(result[key], update[key]);
    }

  } else {
    result = (typeof value === "object" && value !== null) ? assign({}, value) : {};

    for (const key of Object.keys(update)) {
      const value = update[key];
      if (Array.isArray(value) && value.length === 0) {
        delete result[key];
      } else {
        result[key] = merge(result[key], value);
      }
    }
  }

  return Object.freeze(result);
}

function dataFromSubscription(subscription) {
  return Object.freeze({
    state: subscription.state,
    value: subscription.value,
    error: subscription.error,
    valid: subscription.state === "subscribed" || subscription.state === "cached"
  });
}

class LRUCache {
  constructor(size) {
    this.size = size;
    this.cache = [];
  }

  add(key, value) {
    for (let i = 0; i < this.cache.length; i++) {
      if (isEqual(this.cache[i].key, key)) {
        this.cache.splice(i, 1);
        break;
      }
    }

    this.cache.unshift({key, value});

    while (this.cache.length > this.size) {
      this.cache.pop();
    }
  }

  find(k) {
    for (const {key, value} of this.cache) {
      if (isEqual(key, k)) {
        return value;
      }
    }
    return undefined;
  }
}

export class Connection extends BaseConnection {
  constructor(getUrl, options) {
    super(getUrl, options);

    this.nextId = 0;
    this.subscriptions = new Map();
    this.mutations = new Map();

    this.past = new LRUCache(100);

    this.initialRetryDelay = 1000;
    this.maxRetryDelay = 60000;
  }

  makeId() {
    return (this.nextId++).toString();
  }

  subscribe({query, variables, observer}) {
    const id = this.makeId();

    const cached = this.past.find({query, variables});

    const subscription = {
      state: cached ? "cached" : "loading",
      retryDelay: this.initialRetryDelay,
      retryHandle: undefined,
      query,
      variables,
      observer,
      value: cached,
      error: undefined,
    };

    this.subscriptions.set(id, subscription);

    if (this.state === "connected") {
      this.send({id, type: "subscribe", message: {query, variables}});
    }

    return {
      close: () => {
        const subscription = this.subscriptions.get(id);
        if (!subscription) {
          return;
        }

        if (subscription.value !== undefined) {
          this.past.add({query, variables}, subscription.value);
        }

        this.subscriptions.delete(id);
        if (this.state === "connected") {
          this.send({id, type: "unsubscribe"});
        }
      },
      data: () => {
        return dataFromSubscription(subscription);
      }
    };
  }

  mutate({query, variables}) {
    const id = this.makeId();

    if (this.state === "connected") {
      this.send({id, type: "mutate", message: {query, variables}});
    } else {
      return Promise.reject(new Error("not connected"));
    }

    const mutation = {
      query,
      variables,
    };

    const promise = new Promise((resolve, reject) => {
      mutation.resolve = resolve;
      mutation.reject = reject;
    });
    mutation.timeout = setTimeout(
      () => mutation.reject(new Error("mutation timed out")),
      mutationTimeout);

    this.mutations.set(id, mutation);
    return promise;
  }

  notify(subscription) {
    subscription.observer(dataFromSubscription(subscription));
  }

  retry(id) {
    const subscription = this.subscriptions.get(id);
    if (subscription === undefined) {
      return;
    }

    this.send({id, type: "subscribe", message: {query: subscription.query, variables: subscription.variables}});
  }

  handleOpen() {
    for (const [id, subscription] of this.subscriptions) {
      this.send({id, type: "subscribe", message: {query: subscription.query, variables: subscription.variables}});
      if (subscription.retryHandle) {
        clearTimeout(subscription.retryHandle);
        subscription.retryHandle = undefined;
      }
    }
  }

  handleClose() {
    for (const [_, subscription] of this.subscriptions) {
      if (subscription.state === "subscribed") {
        subscription.state = "cached";
        this.notify(subscription);
      }
    }

    for (const [_, mutation] of this.mutations) {
      mutation.reject(new Error("connection closed"));
    }
    this.mutations.clear();
  }

  handleMessage(envelope) {
    let subscription, mutation;
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
        subscription.retryDelay = Math.min(this.maxRetryDelay,
            subscription.retryDelay * 2);
        this.notify(subscription);
      }

      mutation = this.mutations.get(envelope.id);
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
      break;

    default:
      break;
    }
  }
}
