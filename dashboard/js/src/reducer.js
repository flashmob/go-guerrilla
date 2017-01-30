import Immutable from 'immutable';
import ActionTypes from './action-types';
console.log(ActionTypes);
const initialState = Immutable.Map({
	// Keep enough points for 24hrs at 30s resolution
	maxPoints: 24 * 60 * 2,
	// List of points for time series charts
	ram: Immutable.List(),
	nClients: Immutable.List()
});

const reducer = (state = initialState, {type, payload}) => {
	let newState = state;

	switch (type) {
	// Upon establishing a websocket connection, initiates store with dump
	// of last N points, up to `maxPoints`
	// payload = {ram: [{x, y}], nClients: [{x, y}]}
	case ActionTypes.INIT:
		newState = state
			.set('ram', state.get('ram')
				.push(...payload.ram))
			.set('nClients', state.get('nClients')
				.push(...payload.nClients));
		if (newState.get('ram').count() > state.get('maxPoints')) {
			newState = newState
				.set('ram', state.get('ram')
					.shift())
				.set('nClients', state.get('nClients')
					.shift());
		}
		return newState;

	// Updates store with a tick from websocket connection, one point for each
	// chart. Removes oldest point if necessary to make space.
	// payload = {ram: {x, y}, nClients: {x, y}}
	case ActionTypes.TICK:
		newState = state
			.set('ram', state.get('ram')
				.push(payload.ram))
			.set('nClients', state.get('nClients')
				.push(payload.nClients));
		if (newState.get('ram').count() > state.get('maxPoints')) {
			newState = newState
				.set('ram', state.get('ram')
					.shift())
				.set('nClients', state.get('nClients')
					.shift());
		}
		return newState;

	default:
		return state;
	}
}

export default reducer;
