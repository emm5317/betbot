(() => {
    const formatRelative = (value) => {
        const timestamp = new Date(value);
        if (Number.isNaN(timestamp.getTime())) {
            return value;
        }

        const seconds = Math.round((timestamp.getTime() - Date.now()) / 1000);
        const abs = Math.abs(seconds);

        if (abs < 60) {
            return seconds >= 0 ? `in ${abs}s` : `${abs}s ago`;
        }

        const minutes = Math.round(abs / 60);
        if (minutes < 60) {
            return seconds >= 0 ? `in ${minutes}m` : `${minutes}m ago`;
        }

        const hours = Math.round(minutes / 60);
        if (hours < 48) {
            return seconds >= 0 ? `in ${hours}h` : `${hours}h ago`;
        }

        const days = Math.round(hours / 24);
        return seconds >= 0 ? `in ${days}d` : `${days}d ago`;
    };

    const syncTimestamps = () => {
        document.querySelectorAll('.timestamp[data-timestamp]').forEach((node) => {
            const raw = node.getAttribute('data-timestamp');
            if (!raw || raw === '-' || raw === 'pending') {
                return;
            }

            node.setAttribute('title', raw);
            node.textContent = formatRelative(raw);
        });
    };

    document.addEventListener('DOMContentLoaded', () => {
        syncTimestamps();
        window.setInterval(syncTimestamps, 30000);
    });
})();
