export type Omit<T, K extends keyof T> = Pick<T, Exclude<keyof T, K>>;
export type Overwrite<T, U> = Omit<T, Extract<keyof T, keyof U>> & U;
