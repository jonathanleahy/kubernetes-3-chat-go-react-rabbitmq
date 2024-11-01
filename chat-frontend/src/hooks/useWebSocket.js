// src/hooks/useWebSocket.js
import { useState, useEffect, useRef, useCallback } from 'react';

const useWebSocket = () => {
    const [connected, setConnected] = useState(false);
    const [messages, setMessages] = useState([]);
    const wsRef = useRef(null);
    const reconnectAttempts = useRef(0);
    const maxReconnectAttempts = 5;

    const connect = useCallback(() => {
        try {
            const wsUrl = `ws://192.168.49.2:30090/ws`;
            console.log('Connecting to WebSocket at:', wsUrl);

            wsRef.current = new WebSocket(wsUrl);

            wsRef.current.onopen = () => {
                console.log('WebSocket connection opened');
                setConnected(true);
                reconnectAttempts.current = 0;
            };

            wsRef.current.onclose = (event) => {
                console.log('WebSocket connection closed:', event);
                setConnected(false);

                if (reconnectAttempts.current < maxReconnectAttempts) {
                    const timeout = Math.min(1000 * Math.pow(2, reconnectAttempts.current), 10000);
                    console.log(`Reconnecting in ${timeout}ms (attempt ${reconnectAttempts.current + 1}/${maxReconnectAttempts})`);
                    setTimeout(connect, timeout);
                    reconnectAttempts.current++;
                }
            };

            wsRef.current.onerror = (error) => {
                console.error('WebSocket error:', error);
            };

            wsRef.current.onmessage = (event) => {
                console.log('Raw message received:', event.data);
                try {
                    const message = JSON.parse(event.data);
                    console.log('Parsed message:', message);
                    setMessages(prev => {
                        console.log('Previous messages:', prev);
                        const newMessages = [...prev, message];
                        console.log('New messages array:', newMessages);
                        return newMessages;
                    });
                } catch (error) {
                    console.error('Error parsing message:', error);
                }
            };
        } catch (error) {
            console.error('Error setting up WebSocket:', error);
        }
    }, []);

    useEffect(() => {
        connect();
        return () => {
            if (wsRef.current) {
                wsRef.current.close();
            }
        };
    }, [connect]);

    const sendMessage = useCallback((content) => {
        if (wsRef.current?.readyState === WebSocket.OPEN) {
            console.log('Sending message:', content);
            const message = {
                content,
                timestamp: new Date().toISOString()
            };
            wsRef.current.send(JSON.stringify(message));
            console.log('Message sent successfully');
            return true;
        }
        console.log('Cannot send message - WebSocket not connected');
        return false;
    }, []);

    return { connected, messages, sendMessage };
};

export default useWebSocket;