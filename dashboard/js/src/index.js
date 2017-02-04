import React from 'react';
import ReactDOM from 'react-dom';
import App from './components/App';
import reducer from './reducer';
import {applyMiddleware, createStore} from 'redux';
import {Provider} from 'react-redux';
import createLogger from 'redux-logger';
import './index.css';

const logger = createLogger({
	stateTransformer: state => state.toJS()
});

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
