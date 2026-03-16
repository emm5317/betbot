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

    const syncTimestamps = (scope = document) => {
        scope.querySelectorAll('.timestamp[data-timestamp]').forEach((node) => {
            const raw = node.getAttribute('data-timestamp');
            if (!raw || raw === '-' || raw === 'pending' || raw === 'disabled') {
                return;
            }

            node.setAttribute('title', raw);
            node.textContent = formatRelative(raw);
        });
    };

    const installHTMXHooks = () => {
        if (!window.htmx || !document.body) {
            return;
        }

        document.body.addEventListener('htmx:afterSwap', (event) => {
            if (event?.detail?.target) {
                syncTimestamps(event.detail.target);
            }
        });

        document.body.addEventListener('htmx:responseError', (event) => {
            const target = event?.detail?.target;
            if (!target) {
                return;
            }
            target.classList.add('live-fragment--error');
        });
    };

    document.addEventListener('DOMContentLoaded', () => {
        syncTimestamps();
        installHTMXHooks();
        window.setInterval(() => syncTimestamps(), 30000);
    });
})();

// Alpine.js payout calculator for bet entry form
function payoutCalc() {
    return {
        odds: '',
        stake: '',
        potentialWin: 0,
        potentialProfit: 0,
        calc() {
            const o = parseInt(this.odds, 10);
            const s = parseFloat(this.stake);
            if (!o || !s || s <= 0 || (o > -100 && o < 100)) {
                this.potentialWin = 0;
                this.potentialProfit = 0;
                return;
            }
            let profit;
            if (o > 0) {
                profit = s * (o / 100);
            } else {
                profit = s * (100 / Math.abs(o));
            }
            this.potentialProfit = Math.round(profit * 100) / 100;
            this.potentialWin = Math.round((s + profit) * 100) / 100;
        },
        updateSport(event) {
            const option = event.target.selectedOptions[0];
            if (option && option.dataset.sport) {
                document.getElementById('sport').value = option.dataset.sport;
            }
        }
    };
}
