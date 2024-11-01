// src/components/ChatRoom.js
import React, { useEffect, useRef, useState } from 'react';
import Message from './Message';
import MessageInput from './MessageInput';
import useWebSocket from '../hooks/useWebSocket';

const ChatRoom = () => {
    const messagesEndRef = useRef(null);
    const { connected, messages, sendMessage } = useWebSocket();
    const [currentPod, setCurrentPod] = useState('');
    const [userCount, setUserCount] = useState(0);

    // Auto-scroll to bottom when new messages arrive
    useEffect(() => {
        messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
    }, [messages]);

    // Handle system messages and updates
    useEffect(() => {
        const systemMsg = messages.find(m => m.type === 'system');
        if (systemMsg?.podName) {
            setCurrentPod(systemMsg.podName);
        }
        console.log('Current messages:', messages);
    }, [messages]);

    // Fetch user count periodically
    useEffect(() => {
        const fetchUserCount = async () => {
            try {
                const response = await fetch(`http://192.168.49.2:30090/status`);
                const data = await response.json();
                console.log('Status response:', data);
                setUserCount(data.clientCount);
            } catch (error) {
                console.error('Error fetching user count:', error);
            }
        };

        if (connected) {
            fetchUserCount();
            const interval = setInterval(fetchUserCount, 5000);
            return () => clearInterval(interval);
        }
    }, [connected]);

    const handleSendMessage = (content) => {
        console.log('Attempting to send message:', content);
        if (sendMessage(content)) {
            console.log('Message sent successfully');
        } else {
            console.log('Failed to send message');
        }
    };

    return (
        <div className="h-full max-w-2xl mx-auto">
            <div className="bg-white shadow-lg rounded-lg flex flex-col h-full">
                <div className="px-6 py-3 bg-blue-600 rounded-t-lg">
                    <div className="flex items-center justify-between">
                        <div className="flex items-center">
                            <div className={`w-2 h-2 rounded-full mr-2 ${connected ? 'bg-green-400' : 'bg-red-400'}`}></div>
                            <span className="text-white text-sm">
                {connected ? 'Connected' : 'Disconnected - trying to reconnect...'}
              </span>
                        </div>
                        <div className="flex items-center space-x-4">
                            <div className="text-white text-sm">
                                {userCount} {userCount === 1 ? 'user' : 'users'} online
                            </div>
                            {currentPod && (
                                <div className="text-white text-sm bg-blue-700 px-2 py-1 rounded">
                                    Backend: {currentPod}
                                </div>
                            )}
                        </div>
                    </div>
                </div>

                <div className="flex-1 p-4 overflow-y-auto messages-container">
                    {messages.map((msg, index) => (
                        <Message
                            key={index}
                            message={msg}
                            showPodInfo={true}
                        />
                    ))}
                    <div ref={messagesEndRef} />
                </div>

                <MessageInput onSendMessage={handleSendMessage} disabled={!connected} />
            </div>
        </div>
    );
};

export default ChatRoom;