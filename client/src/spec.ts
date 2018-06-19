export interface MutationSpec<
  Result extends object,
  Input extends object | undefined = undefined
> {
  query: string;
  result?: Result;
  variables?: Input;
}

export interface QuerySpec<
  Result extends object,
  Input extends object | undefined = undefined
> {
  query: string;
  result?: Result;
  variables?: Input;
}
