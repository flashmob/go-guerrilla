// Takes a list of strings representing action types and returns an object
// mapping each string to itself. For instance: ['a', 'b'] => {a: 'a', b: 'b'}
export const createActionTypes = (list) => {
	const types = {};
	for (let i = 0; i < list.length; i++) {
		types[list[i]] = list[i];
	}
	return types;
};
