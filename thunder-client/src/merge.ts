export function merge(value: any, update: any): any {
  if (Array.isArray(update)) {
    if (typeof update[0] === "object") {
      return Object.freeze(update[0]);
    } else {
      return update[0];
    }
  }

  if (Array.isArray(update)) {
    return Object.freeze(update.map(elem => merge(null, elem)));
  }

  if (typeof update !== "object" || update === null) {
    return update;
  }

  let result;
  if (Array.isArray(value)) {
    result = [];
    for (let x of (update.$ || [[0, value.length]])) {
      if (Array.isArray(x)) {
        for (let i = x[0]; i < x[0] + x[1]; i++) {
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
      result[key as any] = merge(result[key as any], update[key]);
    }

  } else {
    result = (typeof value === "object" && value !== null) ? Object.assign({}, value) : {};

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
