/**
 * WebSocket Transport Implementation for Wails
 *
 * This is a custom transport that replaces the default HTTP fetch transport
 * with WebSocket-based communication.
 *
 * VERSION 5 - SIMPLIFIED
 */

console.log('[WebSocket Transport] Loading VERSION 5 - simplified');

import { clientId, Base64JSONCodec } from '/wails/runtime.js';

/**
 * Generate a unique ID (simplified nanoid implementation)
 */
function nanoid(size = 21) {
    const alphabet = 'useandom-26T198340PX75pxJACKVERYMINDBUSHWOLF_GQZbfghjklqvwyzrict';
    let id = '';
    let i = size;
    while (i--) {
        id += alphabet[(Math.random() * 64) | 0];
    }
    return id;
}

/**
 * WebSocket Transport class
 */
export class WebSocketTransport {
    constructor(url, options = {}) {
        this.url = url;
        this.ws = null;
        this.wsReady = false;
        this.pendingRequests = new Map();
        this.messageQueue = [];
        this.reconnectTimer = null;
        this.reconnectDelay = options.reconnectDelay || 2000;
        this.requestTimeout = options.requestTimeout || 30000;
        this.codec = options.codec || new Base64JSONCodec();

        this.connect();
    }

    /**
     * Connect to the WebSocket server
     */
    connect() {
        if (this.ws?.readyState === WebSocket.OPEN) {
            return Promise.resolve();
        }

        return new Promise((resolve, reject) => {
            this.ws = new WebSocket(this.url);

            this.ws.onopen = () => {
                console.log(`[WebSocket] ✓ Connected to ${this.url}`);
                this.wsReady = true;

                // Send queued messages
                while (this.messageQueue.length > 0) {
                    const msg = this.messageQueue.shift();
                    this.ws.send(JSON.stringify(msg));
                }

                resolve();
            };

            this.ws.onmessage = async (event) => {
                // Handle both text and binary messages
                let data = event.data;
                if (data instanceof Blob) {
                    data = await data.text();
                }
                this.handleMessage(data);
            };

            this.ws.onerror = (error) => {
                console.error('[WebSocket] Error:', error);
                this.wsReady = false;
                reject(error);
            };

            this.ws.onclose = () => {
                console.log('[WebSocket] Disconnected');
                this.wsReady = false;

                // Reject all pending requests
                this.pendingRequests.forEach(({ reject, timeout }) => {
                    clearTimeout(timeout);
                    reject(new Error('WebSocket connection closed'));
                });
                this.pendingRequests.clear();

                // Attempt to reconnect
                if (!this.reconnectTimer) {
                    this.reconnectTimer = setTimeout(() => {
                        this.reconnectTimer = null;
                        console.log('[WebSocket] Attempting to reconnect...');
                        this.connect().catch(err => {
                            console.error('[WebSocket] Reconnection failed:', err);
                        });
                    }, this.reconnectDelay);
                }
            };
        });
    }

    /**
     * Handle incoming WebSocket message
     */
    handleMessage(data) {
        console.log('[WebSocket] Received raw message:', data);
        try {
            const msg = JSON.parse(data);
            console.log('[WebSocket] Parsed message:', msg);

            if (msg.type === 'response' && msg.id) {
                const pending = this.pendingRequests.get(msg.id);
                if (!pending) {
                    console.warn('[WebSocket] No pending request for ID:', msg.id);
                    return;
                }

                this.pendingRequests.delete(msg.id);
                clearTimeout(pending.timeout);

                const response = msg.response;
                if (!response) {
                    pending.reject(new Error('Invalid response: missing response field'));
                    return;
                }

                console.log('[WebSocket] Response statusCode:', response.statusCode);

                if (response.statusCode === 200) {
                    // Decode response data using codec
                    let responseData = '';
                    if (response.data) {
                        try {
                            responseData = this.codec.decodeResponse(response.data, response.contentType);
                        } catch (err) {
                            console.error('[WebSocket] Failed to decode response data:', err);
                            pending.reject(new Error('Failed to decode response: ' + err.message));
                            return;
                        }
                    }

                    console.log('[WebSocket] Response data:', responseData);
                    console.log('[WebSocket] Content type:', response.contentType);

                    // For binding calls (object=0, method=0), we need to call the Wails callback handler
                    // because Call.ByName expects window._wails.callResultHandler to be invoked
                    if (responseData && response.contentType?.includes('application/json')) {
                        console.log('[WebSocket] Calling Wails result handler with JSON data');
                        // Extract call-id from the stored request data
                        const callId = pending.request.args?.['call-id'];
                        console.log('[WebSocket] Extracted call-id:', callId);
                        if (callId && window._wails?.callResultHandler) {
                            console.log('[WebSocket] Invoking callResultHandler');
                            window._wails.callResultHandler(callId, responseData, true);
                        }
                        pending.resolve(); // Resolve the transport promise
                    } else {
                        pending.resolve(responseData || undefined);
                    }
                } else {
                    // Decode error data using codec
                    const errorData = this.codec.decodeError(response.data);
                    console.error('[WebSocket] Error response:', errorData);
                    pending.reject(new Error(errorData));
                }
            } else if (msg.type === 'event') {
                console.log('[WebSocket] Received server event:', msg);
                // Dispatch to Wails event system
                if (msg.event && window._wails?.dispatchWailsEvent) {
                    window._wails.dispatchWailsEvent(msg.event);
                    console.log('[WebSocket] Event dispatched to Wails:', msg.event.name);
                }
            }
        } catch (err) {
            console.error('[WebSocket] Failed to parse WebSocket message:', err);
            console.error('[WebSocket] Raw message that failed:', data);
        }
    }

    /**
     * Send a runtime call over WebSocket
     * Implements the RuntimeTransport.call() interface
     */
    async call(objectID, method, windowName, args) {
        // Ensure WebSocket is connected
        if (!this.wsReady) {
            await this.connect();
        }

        return new Promise((resolve, reject) => {
            const msgID = nanoid();

            // Set up timeout
            const timeout = setTimeout(() => {
                if (this.pendingRequests.has(msgID)) {
                    this.pendingRequests.delete(msgID);
                    reject(new Error(`Request timeout (${this.requestTimeout}ms)`));
                }
            }, this.requestTimeout);

            // Register pending request with the message for later reference
            this.pendingRequests.set(msgID, { resolve, reject, timeout, request: { object: objectID, method, args } });

            // Build message
            const message = {
                id: msgID,
                type: 'request',
                request: {
                    object: objectID,
                    method: method,
                    args: args ? JSON.stringify(args) : undefined,
                    windowName: windowName || undefined,
                    clientId: clientId
                }
            };

            // Send or queue message
            if (this.wsReady && this.ws?.readyState === WebSocket.OPEN) {
                this.ws.send(JSON.stringify(message));
            } else {
                this.messageQueue.push(message);
                this.connect().catch(reject);
            }
        });
    }

    /**
     * Close the WebSocket connection
     */
    close() {
        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
            this.reconnectTimer = null;
        }

        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }

        this.wsReady = false;
    }

    /**
     * Get connection status
     */
    isConnected() {
        return this.wsReady && this.ws?.readyState === WebSocket.OPEN;
    }
}

/**
 * Create and configure a WebSocket transport
 *
 * @param url - WebSocket URL (e.g., 'ws://localhost:9998/wails/ws')
 * @param options - Optional configuration
 * @returns WebSocketTransport instance
 */
export function createWebSocketTransport(url, options = {}) {
    return new WebSocketTransport(url, options);
}
