import { isEqual } from "lodash";

/**
 * LRUCache implements a basic LRU uing lodash's isEqual to deeply compare keys.
 */
export class LRUCache<K, V> {
  private readonly size: number;
  private readonly cache: Array<{ key: K; value: V }>;

  constructor(size: number) {
    this.size = size;
    this.cache = [];
  }

  add(key: K, value: V) {
    for (let i = 0; i < this.cache.length; i++) {
      if (isEqual(this.cache[i].key, key)) {
        this.cache.splice(i, 1);
        break;
      }
    }

    this.cache.unshift({ key, value });

    while (this.cache.length > this.size) {
      this.cache.pop();
    }
  }

  find(k: K): V | undefined {
    for (const { key, value } of this.cache) {
      if (isEqual(key, k)) {
        return value;
      }
    }
    return undefined;
  }
}
