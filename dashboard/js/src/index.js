import React from 'react';
import ReactDOM from 'react-dom';
import App from './components/App';
import reducer from './reducer';
import {applyMiddleware, createStore} from 'redux';
import {Provider} from 'react-redux';
import logger from 'redux-logger';
import './index.css';

const store = createStore(
	reducer,
	applyMiddleware(logger)
);

ReactDOM.render(
	<Provider store={store}>
		<App />
	</Provider>,
	document.getElementById('root')
);
