import React from 'react';
import ReactDOM from 'react-dom';
import GraphiQL from 'graphiql';

import './index.css';

import { Client, connectGraphQL as baseConnectGraphQL } from "thunder-client";

export const connection = new Client("ws://localhost:3030/graphql");
export const mutate = connection.mutate.bind(connection);
export const connectGraphQL = baseConnectGraphQL.bind(undefined, connection);

class Editor extends React.PureComponent {
  constructor(props) {
    super(props);
    this.state = {text: ''};
    this.onSubmit = this.onSubmit.bind(this);
  }

  onSubmit(e) {
    mutate({
      query: '{ addMessage(text: $text) }',
      variables: { text: this.state.text },
    }).then(() => {
      this.setState({text: ''});
    });
  }

  render() {
    return (
      <div>
        <input type="text" value={this.state.text} onChange={e => this.setState({text: e.target.value})} />
        <button onClick={this.onSubmit}>Submit</button>
      </div>
    );
  }
}

function deleteMessage(id) {
  mutate({
    query: '{ deleteMessage(id: $id) }',
    variables: { id },
  });
}

function addReaction(messageId, reaction) {
  mutate({
    query: '{ addReaction(messageId: $messageId, reaction: $reaction) }',
    variables: { messageId, reaction },
  });
}

let Messages = (props) => {
  return (
    <div>
      {props.data.value.messages.map(({id, text, reactions}) =>
        <p key={id}>{text}
          <button onClick={() => deleteMessage(id)}>X</button>
          {reactions.map(({reaction, count}) =>
            <button key={reaction} onClick={() => addReaction(id, reaction)}>{reaction} x{count}</button>
          )}
        </p>
      )}
      <Editor />
    </div>
  );
}

Messages = connectGraphQL(Messages, () => ({
  query: `
  {
    messages {
      id, text
      reactions { reaction count }
    }
  }`,
  variables: {},
  onlyValidData: true,
}));

function graphQLFetcher({query, variables}) {
  return {
    subscribe(subscriber) {
      const next = subscriber.next || subscriber;

      const subscription = connection.subscribe({
        query: query,
        variables: {},
        observer: ({state, valid, error, value}) => {
          if (valid) {
            next({data: value});
          } else {
            next({state, error});
          }
        }
      });

      return {
        unsubscribe() {
          return subscription.close();
        }
      };
    }
  };
}

function App() {
  if (window.location.pathname === "/graphiql") {
    return <GraphiQL fetcher={graphQLFetcher} />;
  } else {
    return <Messages />;
  }
}

ReactDOM.render(
  <App />,
  document.getElementById('root')
);
