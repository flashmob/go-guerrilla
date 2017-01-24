import React from 'react';
import ReactDOM from 'react-dom';
import App from './App';
import './index.css';

ReactDOM.render(
  <App />,
  document.getElementById('root')
);

var conn = new WebSocket('ws://localhost:8080/ws');
conn.onclose = function(event) {
	console.log(event);
}

conn.onmessage = function(event) {
	console.log(JSON.parse(event.data));
	var point = JSON.parse(event.data);
	// ram.append(new Date(point.t).getTime(), point.y);
}
