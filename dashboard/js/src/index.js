import React from 'react';
import ReactDOM from 'react-dom';
import App from './components/App';
import reducer from './reducer';
import { applyMiddleware, createStore } from 'redux';
import { Provider } from 'react-redux';
import createLogger from 'redux-logger';
import './index.css';

let store = createStore(reducer);

if (process.env.NODE_ENV === 'development') {
	store = createStore(
		reducer, applyMiddleware(createLogger({
			stateTransformer: state => state.toJS()
		})
	));
}


ReactDOM.render(
	<Provider store={store}>
		<App />
	</Provider>,
	document.getElementById('root')
);
