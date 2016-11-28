import { Client, Handle } from "./client";
import { isEqual } from "lodash";
import * as React from "react";

export function connectGraphQL<P>(client: Client, Component: React.ComponentClass<P>, queryVariablesFunc: (props: any) => {query: string, variables: any, onlyValidData?: boolean}) : React.ComponentClass<P> {
  return class extends React.Component<P, {data: any, query?: string, variables?: any, previous?: any}> {
    static displayName = (Component.displayName || Component.name) + "Connector";

    subscription?: Handle;

    constructor(props: P) {
      super(props);
      this.state = { data: {} };
    }

    componentWillMount() {
      const {query, variables} = queryVariablesFunc(this.props);
      this.subscribe({query, variables});

      // if (process.env.NODE_ENV !== "production") {
        // this.subscribes = 0;
      // }
    }

    componentWillReceiveProps(nextProps: any) {
      const { query, variables } = queryVariablesFunc(nextProps);
      if (isEqual(query, this.state.query) && isEqual(variables, this.state.variables)) {
        return;
      }
      this.subscribe({ query, variables });
    }

    componentWillUnmount() {
      this.unsubscribe();
    }

    subscribe({query, variables}: {query: string, variables: any}) {
      let data = this.state.data;
      if (data.state === "pending" && this.state.previous) {
        data = this.state.previous;
      }

      const previous = this.state.query === query ?
        Object.assign({}, data, { state: "previous" }) : undefined;

      this.unsubscribe();
      this.subscription = client.subscribe({
        query,
        variables,
        observer: data => this.setState({ data }),
      });

      this.setState({ query, variables, previous, data: this.subscription.data() });

      /*
      if (process.env.NODE_ENV !== "production") {
        this.subscribes++;
        setTimeout(() => { this.subscribes--; }, 60 * 1000);
        if (this.subscribes > 5) {
          console.log("WARNING: ", Component.displayName + " has subscribed 5 times in the last minute.");
        }
      }
      */
    }

    unsubscribe() {
      if (this.subscription) {
        this.subscription.close();
        this.subscription = undefined;
      }
    }

    render() {
      let data = this.state.data;
      if (data.state === "pending" && this.state.previous) {
        data = this.state.previous;
      }
      const { onlyValidData } = queryVariablesFunc(this.props);
      if (onlyValidData && !data.valid) {
        return null;
      }
      return React.createElement(Component,  Object.assign({ data }, this.props));
    }
  };
}