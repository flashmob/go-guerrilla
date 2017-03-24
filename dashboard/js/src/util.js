// Takes a list of strings representing action types and returns an object
// mapping each string to itself. For instance: ['a', 'b'] => {a: 'a', b: 'b'}
export const createActionTypes = list => {
	const types = {};
	for (let i = 0; i < list.length; i++) {
		types[list[i]] = list[i];
	}
	return types;
};

export const formatBytes = (bytes, decimals) => {
	if (bytes < 1000) return `${bytes} B`;
	const k = 1000;
	const dm = decimals || 3;
	const sizes = ['B', 'KB', 'MB', 'GB'];
	const i = Math.floor(Math.log(bytes) / Math.log(k));

	if (i < 0) return '';
	return `${parseFloat((bytes / Math.pow(k, i)).toFixed(dm))} ${sizes[i]}`;
};

export const formatNumber = (num, decimals) => {
	if (num < 1000) return `${num}`;
	const k = 1000;
	const dm = decimals || 3;
	const sizes = ['', 'K', 'M', 'B'];
	const i = Math.floor(Math.log(num) / Math.log(k));

	if (i < 0) return '';
	return `${parseFloat((num / Math.pow(k, i)).toFixed(dm))} ${sizes[i]}`;
}
