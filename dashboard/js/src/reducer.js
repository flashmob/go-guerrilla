import Immutable from 'immutable';
import ActionTypes from './action-types';

const initialState = Immutable.Map({
	// Keep enough points for 24hrs at 30s resolution
	maxPoints: 24 * 60 * 2,
	// List of points for time series charts
	ram: Immutable.Map({
		data: Immutable.List(),
		min: Infinity,
		max: -Infinity
	}),
	nClients: Immutable.Map({
		data: Immutable.List(),
		min: Infinity,
		max: -Infinity
	}),
	topDomain: Immutable.Map(),
	topHelo: Immutable.Map(),
	topIP: Immutable.Map()
});

const reducer = (state = initialState, {type, payload}) => {
	let newState = state;

	switch (type) {
	// Upon establishing a websocket connection, initiates store with dump
	// of last N points, up to `maxPoints`
	// payload = {ram: [{x, y}], nClients: [{x, y}]}
	case ActionTypes.INIT:
		payload.ram.forEach(p => {
			if (p.y < state.getIn(['ram', 'min'])) {
				newState = newState.setIn(['ram', 'min'], p.y);
			}
			if (p.y > state.getIn(['ram', 'max'])) {
				newState = newState.setIn(['ram', 'max'], p.y);
			}
		});
		newState = newState
			.setIn(['ram', 'data'], state.getIn(['ram', 'data'])
				.push(...payload.ram))
			.setIn(['nClients', 'data'], state.getIn(['nClients', 'data'])
				.push(...payload.nClients))
			.set('topDomain', Immutable.fromJS(payload.topDomain))
			.set('topHelo', Immutable.fromJS(payload.topHelo))
			.set('topIP', Immutable.fromJS(payload.topIP));
		if (newState.getIn(['ram', 'data']).count() > state.get('maxPoints')) {
			newState = newState
				.setIn(['ram', 'data'], state.getIn(['ram', 'data'])
					.shift())
				.setIn(['nClients', 'data'], state.getIn(['nClients', 'data'])
					.shift());
		}
		return newState;

	// Updates store with a tick from websocket connection, one point for each
	// chart. Removes oldest point if necessary to make space.
	// payload = {ram: {x, y}, nClients: {x, y}}
	case ActionTypes.TICK:
		if (payload.ram.y < state.getIn(['ram', 'min'])) {
			newState = newState.setIn(['ram', 'min'], payload.ram.y);
		}
		if (payload.ram.y > state.getIn(['ram', 'max'])) {
			newState = newState.setIn(['ram', 'max'], payload.ram.y);
		}
		newState = state
			.setIn(['ram', 'data'], state.getIn(['ram', 'data'])
				.push(payload.ram))
			.setIn(['nClients', 'data'], state.getIn(['nClients', 'data'])
				.push(payload.nClients))
			.set('topDomain', Immutable.fromJS(payload.topDomain))
			.set('topHelo', Immutable.fromJS(payload.topHelo))
			.set('topIP', Immutable.fromJS(payload.topIP));
		if (newState.getIn(['ram', 'data']).count() > state.get('maxPoints')) {
			newState = newState
				.setIn(['ram', 'data'], state.getIn(['ram', 'data'])
					.shift())
				.setIn(['nClients', 'data'], state.getIn(['nClients', 'data'])
					.shift());
		}
		return newState;

	default:
		return state;
	}
}

export default reducer;
