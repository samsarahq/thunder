import React from "react";
import { Connection } from "./graphql";
import { isEqual } from "lodash";

export const connection = new Connection("ws://localhost:3030/graphql");

export function connectGraphQL(Component, queryVariablesFunc) {
  return React.createClass({
    displayName: Component.displayName + "Connector",

    getInitialState() {
      return {data: {}};
    },

    componentWillMount() {
      const {query, variables} = queryVariablesFunc(this.props);
      this.subscribe({query, variables});

      if (process.env.NODE_ENV !== "production") {
        this.subscribes = 0;
      }
    },

    componentWillReceiveProps(nextProps) {
      const {query, variables} = queryVariablesFunc(nextProps);
      if (isEqual(query, this.state.query) &&
          isEqual(variables, this.state.variables)) {
        return;
      }
      this.subscribe({query, variables});
    },

    componentWillUnmount() {
      this.unsubscribe();
    },

    subscribe({query, variables}) {
      let data = this.state.data;
      if (data.state === "pending" && this.state.previous) {
        data = this.state.previous;
      }
      const previous = this.state.query === query ?
        {...data, state: "previous"} : undefined;

      this.unsubscribe();
      this.subscription = connection.subscribe({
        query,
        variables,
        observer: data => this.setState({data})
      });

      this.setState({query, variables, previous, data: this.subscription.data()});

      if (process.env.NODE_ENV !== "production") {
        this.subscribes++;
        setTimeout(() => { this.subscribes--; }, 60 * 1000);
        if (this.subscribes > 5) {
          console.log("WARNING: ", Component.displayName + " has subscribed 5 times in the last minute.");
        }
      }
    },

    unsubscribe() {
      if (this.subscription) {
        this.subscription.close();
        this.subscription = undefined;
      }
    },

    render() {
      let data = this.state.data;
      if (data.state === "pending" && this.state.previous) {
        data = this.state.previous;
      }
      const {onlyValidData} = queryVariablesFunc(this.props);
      if (onlyValidData && !data.valid) {
        return false;
      }
      return <Component data={data} {...this.props} />;
    }
  });
}

export const mutate = connection.mutate.bind(connection);

export default connectGraphQL;
