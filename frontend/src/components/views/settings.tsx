// components/views/settings.tsx (Modified)
import React, { useState, useEffect } from 'react';

interface SettingsViewProps {
    currentUserName: string;
    currentUserId: string;
    storagePath: string;
    onUpdateUserNameInMainProcess: (newName: string) => Promise<boolean>;
    onGenerateNewUserId: () => Promise<void>;
    onSetStoragePath: () => Promise<void>;
}

export const SettingsView: React.FC<SettingsViewProps> = ({
    currentUserName,
    currentUserId,
    storagePath,
    onUpdateUserNameInMainProcess,
    onGenerateNewUserId,
    onSetStoragePath
}) => {
    // Username input state (always editable)
    const [tempUserName, setTempUserName] = useState(currentUserName);

    // Effect to update tempUserName when currentUserName prop changes (e.g., initial load)
    useEffect(() => {
        setTempUserName(currentUserName);
    }, [currentUserName]);

    const handleUserNameChange = (event: React.ChangeEvent<HTMLInputElement>) => {
        setTempUserName(event.target.value);
    };

    const handleSaveUserName = async () => {
        // Trim whitespace from the new username
        const trimmedName = tempUserName.trim();
        if (trimmedName !== '' && trimmedName !== currentUserName) {
            try {
                // Call the prop that sends update to main process
                const success = await onUpdateUserNameInMainProcess(trimmedName);
                if (success) {
                    // The `currentUserName` prop will be updated from App.tsx via IPC,
                    // which will then update `tempUserName` via the useEffect above.
                    console.log("Username saved and sent to main process.");
                } else {
                    console.error("Failed to save username to store.");
                }
            } catch (error) {
                console.error("Error saving username via IPC:", error);
            }
        } else {
            // If name is empty or unchanged, reset to last saved
            setTempUserName(currentUserName);
        }
    };

    const handleGenerateId = async () => {
        // This will trigger the parent App component to update userId
        await onGenerateNewUserId();
    };

    return (
        <div className="flex flex-col h-full p-8 overflow-y-auto">
            <h1 className="text-3xl font-semibold text-white mb-6 text-left">Settings</h1>

            <div className="flex flex-col divide-y divide-gray-800">
                {/* Username Row */}
                <div className="px-4 py-4 hover:bg-gray-500/10 rounded-md transition-colors">
                    <div className="flex flex-col items-start gap-2">
                        <label htmlFor="username" className="text-base font-medium text-gray-200">Username</label>
                        <div className="w-full flex flex-col sm:flex-row items-start sm:items-center gap-3">
                            <input
                                type="text"
                                id="username"
                                value={tempUserName}
                                onChange={handleUserNameChange}
                                onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); void handleSaveUserName(); } }}
                                className="min-w-[280px] sm:min-w-[360px] w-full sm:w-auto px-3 py-2 rounded-md bg-gray-800/40 text-white border border-gray-700 focus:outline-none focus:ring-2 focus:ring-blue-500"
                                placeholder="Enter new username"
                            />
                        </div>
                    </div>
                </div>

                {/* User ID Row */}
                <div className="px-4 py-4 hover:bg-gray-500/10 rounded-md transition-colors">
                    <div className="flex flex-col items-start gap-2">
                        <div className="text-base font-medium text-gray-200">User ID</div>
                        <p className="text-gray-300 text-sm sm:text-base break-all font-mono min-w-[280px] sm:min-w-[360px] w-full sm:w-auto text-left">{currentUserId}</p>
                        <button
                            onClick={handleGenerateId}
                            className="mt-1 px-3 py-2 text-sm rounded-md border border-gray-600 text-gray-200 hover:bg-gray-500/20"
                        >
                            Generate New ID
                        </button>
                    </div>
                </div>

                {/* Storage Path Row */}
                <div className="px-4 py-4 hover:bg-gray-500/10 rounded-md transition-colors">
                    <div className="flex flex-col items-start gap-2">
                        <div className="text-base font-medium text-gray-200">Storage Path</div>
                        <p className="text-gray-300 text-sm sm:text-base break-all font-mono min-w-[280px] sm:min-w-[360px] w-full sm:w-auto text-left">{storagePath}</p>
                        <button
                            onClick={onSetStoragePath}
                            className="mt-1 px-3 py-2 text-sm rounded-md border border-gray-600 text-gray-200 hover:bg-gray-500/20"
                        >
                            Set Storage Path
                        </button>
                    </div>
                </div>

                {/* Profile Picture Row (Placeholder) */}
                <div className="px-4 py-4 hover:bg-gray-500/10 rounded-md transition-colors">
                    <div className="flex flex-col items-start gap-2">
                        <div className="text-base font-medium text-gray-200">Profile Picture</div>
                        <p className="text-gray-400 text-sm min-w-[280px] sm:min-w-[360px] w-full sm:w-auto text-left">Feature to upload or select a profile picture will be available here.</p>
                        <button
                            className="mt-1 px-3 py-2 text-sm rounded-md border border-gray-700 text-gray-500 cursor-not-allowed min-w-[180px] text-left"
                            disabled
                        >
                            Upload Picture (Coming Soon)
                        </button>
                    </div>
                </div>
            </div>
        </div>
    );
};