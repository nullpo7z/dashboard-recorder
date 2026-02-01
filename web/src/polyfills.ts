import { v4 as uuidv4 } from 'uuid';

// Polyfill for randomUUID in non-secure contexts
if (!window.crypto) {
    window.crypto = {} as Crypto
}
if (!window.crypto.randomUUID) {
    window.crypto.randomUUID = () => {
        return uuidv4() as `${string}-${string}-${string}-${string}-${string}`;
    }
}
