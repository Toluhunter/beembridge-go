import { FaFile } from "react-icons/fa";
import { FaFolder } from "react-icons/fa";
import React, { useEffect, useState, useMemo, useCallback } from 'react';
import { useMockTransferEngine } from '../../utils/mockTransfers.js';
import TransferProgressItem from '../shared/transfer-progress-item.js';
import { FaHashtag } from "react-icons/fa";

export interface ActiveTransferDisplayItem extends Progress {
    status: 'pending' | 'in-progress' | 'completed' | 'failed' | 'cancelled';
}

export type Progress = {
    fileId: string;
    fileName: string;
    totalBytes: number;
    transferredBytes: number;
    percentage: number;
    speedKbps?: number; // Optional: speed calculation
    parentId?: string;
    rootDir?: string;
}

interface ActiveTransferViewProps {
    activeTransfers: ActiveTransferDisplayItem[],
    hashingProgress: { [key: string]: number }
}

export const ActiveTransferView: React.FC<ActiveTransferViewProps> = ({ activeTransfers, hashingProgress }) => {
    // State to manage the expanded/collapsed state of parent groups
    const [expandedParents, setExpandedParents] = useState<Record<string, boolean>>({});
    // Pagination state for grouped and individual transfers
    const [groupedPage, setGroupedPage] = useState(1);
    const [groupedPerPage, setGroupedPerPage] = useState<number>(3);
    const [individualPage, setIndividualPage] = useState(1);
    const [individualPerPage, setIndividualPerPage] = useState<number>(6);
    // Pagination state for hashing items
    const [hashingPage, setHashingPage] = useState(1);
    const [hashingPerPage, setHashingPerPage] = useState<number>(6);
    // Enable mocks automatically if no real data is present
    const useMock = activeTransfers.length === 0 && Object.keys(hashingProgress).length === 0;
    const { mockActiveTransfers, mockHashingProgress } = useMockTransferEngine(useMock);

    // Function to determine the color based on status
    const getStatusColor = (status: ActiveTransferDisplayItem['status']) => {
        switch (status) {
            case 'completed': return 'text-green-500';
            case 'failed': return 'text-red-500';
            case 'in-progress': return 'text-blue-500';
            case 'pending': return 'text-yellow-500';
            case 'cancelled': return 'text-gray-400';
            default: return 'text-gray-300';
        }
    };

    // Group transfers by parentId and calculate average percentage
    const renderActiveTransfers = useMock ? mockActiveTransfers : activeTransfers;
    const renderHashingProgress = useMock ? mockHashingProgress : hashingProgress;

    const { groupedTransfers, individualTransfers } = useMemo(() => {
        const grouped: Record<string, ActiveTransferDisplayItem[]> = {};
        const individual: ActiveTransferDisplayItem[] = [];

        renderActiveTransfers.forEach(transfer => {
            if (transfer.parentId && transfer.rootDir) {
                if (!grouped[transfer.rootDir]) {
                    grouped[transfer.rootDir] = [];
                }
                grouped[transfer.rootDir].push(transfer);
            } else {
                individual.push(transfer);
            }
        });
        return { groupedTransfers: grouped, individualTransfers: individual };
    }, [renderActiveTransfers]); // Recalculate when activeTransfers (or mock) changes

    // Dynamically set items per page based on screen HEIGHT (applies to both groups and files)
    const updatePerPage = useCallback(() => {
        if (typeof window === 'undefined') return;
        const h = window.innerHeight;

        // Estimate available vertical space minus header/nav/paddings
        const reserved = 280; // header, margins, hashing section padding estimate
        const available = Math.max(200, h - reserved);
        // Approximate item height (collapsed card or row)
        const itemHeight = 140; // px
        let per = Math.floor(available / itemHeight);
        // Clamp to reasonable bounds
        per = Math.max(1, Math.min(10, per));

        setGroupedPerPage(per);
        setIndividualPerPage(per);
        setHashingPerPage(per);
        // Reset pages to 1 when layout changes to avoid out-of-range pages
        setGroupedPage(1);
        setIndividualPage(1);
        setHashingPage(1);
    }, []);

    useEffect(() => {
        updatePerPage();
        window.addEventListener('resize', updatePerPage);
        return () => window.removeEventListener('resize', updatePerPage);
    }, [updatePerPage]);

    // Function to calculate average percentage for a group
    const getAveragePercentage = (transfers: ActiveTransferDisplayItem[]) => {
        if (transfers.length === 0) return 0;
        const totalPercentage = transfers.reduce((sum, t) => sum + (t.percentage || 0), 0);
        return Math.round(totalPercentage / transfers.length);
    };

    // Function to get the overall status of a group
    const getGroupStatus = (transfers: ActiveTransferDisplayItem[]) => {
        if (transfers.some(t => t.status === 'failed')) return 'failed';
        if (transfers.some(t => t.status === 'in-progress')) return 'in-progress';
        if (transfers.every(t => t.status === 'completed')) return 'completed';
        if (transfers.some(t => t.status === 'pending')) return 'pending';
        return 'pending'; // Default or mixed
    };

    // Toggle dropdown for a parent group
    const toggleParent = (parentId: string) => {
        setExpandedParents(prev => ({
            ...prev,
            [parentId]: !prev[parentId]
        }));
    };

    return (
        <div className="p-6 rounded-xl shadow-lg h-full flex flex-col font-inter">
            <h1 className="text-3xl font-bold text-white mb-6 text-start">Active Transfers</h1>
            <div className="border border-gray-600 rounded-lg p-4 flex flex-col h-full">
                {renderActiveTransfers.length === 0 && Object.keys(renderHashingProgress).length === 0 ? (
                    <div className="flex flex-col items-center justify-center h-full text-center">
                        <img src="/src/assets/images/uploading_nu4x.svg" alt="No active transfers" className="min-w-72 w-full h-50 mb-6" />
                        <p className="text-gray-400 text-lg">No active transfers at the moment.</p>
                    </div>
                ) : (
                    <div className="flex-grow overflow-y-auto custom-scrollbar pr-4">
                        {(() => {
                            const hashingEntries = Object.entries(renderHashingProgress);
                            if (hashingEntries.length === 0) return null;
                            const total = hashingEntries.length;
                            const totalPages = Math.ceil(total / hashingPerPage) || 1;
                            const start = (hashingPage - 1) * hashingPerPage;
                            const end = start + hashingPerPage;
                            const pageItems = hashingEntries.slice(start, end);

                            return (
                                <div className="mb-6">
                                    <h2 className="text-2xl font-bold text-white mb-4">Preparing Files...</h2>
                                    <div className="grid grid-cols-1 gap-0">
                                        {pageItems.map(([filePath, percentage]) => (
                                            <TransferProgressItem
                                                key={filePath}
                                                title={(filePath.split('\\').pop() || filePath)}
                                                percentage={percentage}
                                                status="hashing"
                                                icon={<FaHashtag className="text-gray-400" />}
                                                showCancel={false}
                                            />
                                        ))}
                                    </div>

                                    {totalPages > 1 && (
                                        <div className="col-span-full flex min-w-[285px] justify-between items-center p-2 mt-2 border-t border-gray-700 bg-gray-800/30 rounded">
                                            <div className="text-sm text-gray-400">
                                                Showing {Math.min(start + 1, total)} to {Math.min(end, total)} of {total} hashing items
                                            </div>
                                            <div className="flex items-center space-x-2">
                                                <button
                                                    onClick={() => setHashingPage(prev => Math.max(prev - 1, 1))}
                                                    disabled={hashingPage === 1}
                                                    className="px-2 py-1 text-gray-300 hover:text-white disabled:opacity-50 transition-colors"
                                                    aria-label="Previous Page"
                                                >
                                                    {'<'}
                                                </button>
                                                <div className="text-sm text-gray-300">{hashingPage} / {totalPages}</div>
                                                <button
                                                    onClick={() => setHashingPage(prev => Math.min(prev + 1, totalPages))}
                                                    disabled={hashingPage === totalPages}
                                                    className="px-2 py-1 text-gray-300 hover:text-white disabled:opacity-50 transition-colors"
                                                    aria-label="Next Page"
                                                >
                                                    {'>'}
                                                </button>
                                            </div>
                                        </div>
                                    )}
                                </div>
                            );
                        })()}
                        <div className="grid grid-cols-1 gap-6">
                            {/* Render Grouped Transfers with pagination */}
                            {(() => {
                                const groupedEntries = Object.entries(groupedTransfers);
                                const total = groupedEntries.length;
                                const totalPages = Math.ceil(total / groupedPerPage) || 1;
                                const start = (groupedPage - 1) * groupedPerPage;
                                const end = start + groupedPerPage;
                                const pageItems = groupedEntries.slice(start, end);

                                return (
                                    <>
                                        {pageItems.map(([parentId, children]) => {
                                            const averagePercentage = getAveragePercentage(children);
                                            const groupStatus = getGroupStatus(children);
                                            const isExpanded = expandedParents[parentId];

                                            return (
                                                <div key={parentId} className="p-5 rounded-xl min-w-[430px] shadow-md border border-gray-600 hover:border-blue-500 transition-all duration-200 w-full">
                                                    <div className="flex items-center justify-between cursor-pointer" onClick={() => toggleParent(parentId)}>
                                                        <div className="flex items-start flex-1 min-w-[240px]">
                                                            <span className="text-3xl mr-3 flex-shrink-0">
                                                                <FaFolder className="text-gray-400" />
                                                            </span>
                                                            <div className="flex-1 min-w-0">
                                                                <h3 className="text-xl font-semibold text-white whitespace-normal break-words">
                                                                    Group: {parentId}
                                                                </h3>
                                                                <p className={`text-sm font-medium ${getStatusColor(groupStatus)} capitalize`}>
                                                                    Status: {groupStatus}
                                                                </p>
                                                            </div>
                                                        </div>
                                                        <div className="flex items-center ml-4">
                                                            <span className="ml-2 text-blue-400 font-bold text-lg">{averagePercentage}%</span>
                                                            <span className="text-white ml-2 text-xl">
                                                                {isExpanded ? '▲' : '▼'}
                                                            </span>
                                                        </div>
                                                    </div>

                                                    {isExpanded && (
                                                        <div className="mt-4 border-t border-gray-600 pt-4 pl-8">
                                                            <h4 className="text-lg font-semibold text-white mb-3">Files in this group:</h4>
                                                            {children.map(transfer => (
                                                                <TransferProgressItem
                                                                    key={transfer.fileId}
                                                                    title={transfer.fileName}
                                                                    percentage={transfer.percentage}
                                                                    status={transfer.status}
                                                                    speedKbps={transfer.speedKbps}
                                                                    icon={<FaFile />}
                                                                    showCancel={true}
                                                                    onCancel={() => console.log('Cancel transfer', transfer.fileId)}
                                                                />
                                                            ))}
                                                        </div>
                                                    )}
                                                </div>
                                            );
                                        })}

                                        {totalPages > 1 && (
                                            <div className="col-span-full flex min-w-[285px] justify-between items-center p-2 mt-2 border-t border-gray-700 bg-gray-800/30 rounded">
                                                <div className="text-sm text-gray-400">
                                                    Showing {Math.min(start + 1, total)} to {Math.min(end, total)} of {total} groups
                                                </div>
                                                <div className="flex items-center space-x-2">
                                                    <button
                                                        onClick={() => setGroupedPage(prev => Math.max(prev - 1, 1))}
                                                        disabled={groupedPage === 1}
                                                        className="px-2 py-1 text-gray-300 hover:text-white disabled:opacity-50 transition-colors"
                                                        aria-label="Previous Page"
                                                    >
                                                        {'<'}
                                                    </button>
                                                    <div className="text-sm text-gray-300">{groupedPage} / {totalPages}</div>
                                                    <button
                                                        onClick={() => setGroupedPage(prev => Math.min(prev + 1, totalPages))}
                                                        disabled={groupedPage === totalPages}
                                                        className="px-2 py-1 text-gray-300 hover:text-white disabled:opacity-50 transition-colors"
                                                        aria-label="Next Page"
                                                    >
                                                        {'>'}
                                                    </button>
                                                </div>
                                            </div>
                                        )}
                                    </>
                                );
                            })()}

                            {/* Render Individual Transfers (without parentId) with pagination */}
                            {(() => {
                                const total = individualTransfers.length;
                                const totalPages = Math.ceil(total / individualPerPage) || 1;
                                const start = (individualPage - 1) * individualPerPage;
                                const end = start + individualPerPage;
                                const pageItems = individualTransfers.slice(start, end);

                                return (
                                    <>
                                        {pageItems.map(transfer => (
                                            <div
                                                key={transfer.fileId}
                                                className="group relative p-5 border-b border-gray-600 shadow-md transition-all duration-200 w-full min-w-[430px]"
                                            >
                                                <div className="flex items-center justify-between mb-3 w-full">
                                                    <div className="flex items-start min-w-[240px]">
                                                        <span className="text-3xl mr-3 flex-shrink-0 text-gray-400">
                                                            <FaFile />
                                                        </span>
                                                        <div className="flex-1 min-w-0">
                                                            <h3 className="text-xl font-semibold text-white whitespace-normal break-words">
                                                                {transfer.fileName}
                                                            </h3>
                                                        </div>
                                                    </div>

                                                    {/* percentage and cancel button (X) */}
                                                    {transfer.percentage !== undefined && (
                                                        <div className="ml-4 flex items-center">
                                                            <span className="font-bold transition-opacity duration-200 group-hover:opacity-0">{transfer.percentage.toFixed(0)}%</span>
                                                            <button
                                                                onClick={() => console.log('Cancel transfer', transfer.fileId)}
                                                                className="hidden group-hover:inline-flex ml-2 bg-red-600 text-white rounded px-2 py-0.5 text-sm"
                                                                aria-label={`Cancel ${transfer.fileName}`}
                                                            >
                                                                ✕
                                                            </button>
                                                        </div>
                                                    )}
                                                </div>

                                                {transfer.speedKbps !== undefined && transfer.status === 'in-progress' && (
                                                    <span className="ml-2 text-gray-400 text-sm">({(transfer.speedKbps / 1024).toFixed(2)} MB/s)</span>
                                                )}

                                                {transfer.status === 'in-progress' && transfer.percentage !== undefined && (
                                                    <div className="w-full bg-gray-600 rounded-full h-2.5 mb-3">
                                                        <div
                                                            className="bg-blue-500 h-2.5 rounded-full transition-all duration-500 ease-out"
                                                            style={{ width: `${transfer.percentage}%` }}
                                                        ></div>
                                                    </div>
                                                )}
                                            </div>
                                        ))}

                                        {totalPages > 1 && (
                                            <div className="col-span-full flex min-w-[285px] justify-between items-center p-2 mt-2 border-t border-gray-700 bg-gray-800/30 rounded">
                                                <div className="text-sm text-gray-400">
                                                    Showing {Math.min(start + 1, total)} to {Math.min(end, total)} of {total} files
                                                </div>
                                                <div className="flex items-center space-x-2">
                                                    <button
                                                        onClick={() => setIndividualPage(prev => Math.max(prev - 1, 1))}
                                                        disabled={individualPage === 1}
                                                        className="px-2 py-1 text-gray-300 hover:text-white disabled:opacity-50 transition-colors"
                                                        aria-label="Previous Page"
                                                    >
                                                        {'<'}
                                                    </button>
                                                    <div className="text-sm text-gray-300">{individualPage} / {totalPages}</div>
                                                    <button
                                                        onClick={() => setIndividualPage(prev => Math.min(prev + 1, totalPages))}
                                                        disabled={individualPage === totalPages}
                                                        className="px-2 py-1 text-gray-300 hover:text-white disabled:opacity-50 transition-colors"
                                                        aria-label="Next Page"
                                                    >
                                                        {'>'}
                                                    </button>
                                                </div>
                                            </div>
                                        )}
                                    </>
                                );
                            })()}
                        </div>
                    </div>
                )}
            </div>
        </div>
    );
};
