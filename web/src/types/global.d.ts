export { };

declare global {
    interface Window {
        crypto: Crypto;
    }
}
