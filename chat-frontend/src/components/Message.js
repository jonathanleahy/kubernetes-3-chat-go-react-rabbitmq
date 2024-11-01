// src/components/Message.js
import React from 'react';

const Message = ({ message, showPodInfo }) => {
    const isSystem = message.type === 'system';

    return (
        <div className={`mb-4 ${isSystem ? 'text-center' : ''}`}>
            <div className={`inline-block rounded-lg px-4 py-2 max-w-[80%] ${
                isSystem
                    ? 'bg-gray-100 text-gray-600'
                    : 'bg-blue-100 text-blue-900'
            }`}>
                <p>{message.content}</p>
                <div className="flex justify-between items-center mt-1 text-xs text-gray-500">
                    <span>{new Date(message.timestamp).toLocaleTimeString()}</span>
                    {showPodInfo && message.podName && (
                        <span className={`ml-2 px-2 py-0.5 rounded-full ${
                            isSystem ? 'bg-gray-200' : 'bg-blue-200'
                        }`}>
              Pod: {message.podName}
            </span>
                    )}
                </div>
            </div>
        </div>
    );
};

export default Message;