/**
 * Merge combines a graphql update from the websocket with an existing value
 * into an updated value.
 */
export function merge(original: any, update: any): any {
  if (Array.isArray(update)) {
    if (typeof update[0] === "object") {
      return Object.freeze(update[0]);
    } else {
      return update[0];
    }
  }

  if (typeof update !== "object" || update === null) {
    return update;
  }

  let merged;
  if (Array.isArray(original)) {
    merged = [];
    for (const x of update.$ || [[0, original.length]]) {
      if (Array.isArray(x)) {
        for (let i = x[0]; i < x[0] + x[1]; i++) {
          merged.push(original[i]);
        }
      } else if (merged[x] === -1) {
        merged.push(undefined);
      } else {
        merged.push(original[x]);
      }
    }
    delete update.$;

    for (const key of Object.keys(update)) {
      merged[Number(key)] = merge(merged[Number(key)], update[key]);
    }
  } else {
    merged =
      typeof original === "object" && original !== null ? { ...original } : {};

    for (const key of Object.keys(update)) {
      const value = update[key];
      if (Array.isArray(value) && value.length === 0) {
        delete merged[key];
      } else {
        merged[key] = merge(merged[key], value);
      }
    }
  }

  return Object.freeze(merged);
}
