//! Yeniden başlatma için üstel geri çekilme.

use std::time::Duration;

/// Her `next` çağrısında bekleme süresini ikiye katlar, `max` ile sınırlar.
#[derive(Debug, Clone)]
pub struct Backoff {
    initial: Duration,
    max: Duration,
    current: Duration,
}

impl Backoff {
    pub fn new(initial: Duration, max: Duration) -> Self {
        let initial = initial.min(max);
        Self {
            initial,
            max,
            current: initial,
        }
    }

    /// Sıradaki bekleme süresini döndürür ve bir sonrakini büyütür.
    pub fn next_delay(&mut self) -> Duration {
        let delay = self.current;
        self.current = self.current.saturating_mul(2).min(self.max);
        delay
    }

    /// Süreç bir süre sağlıklı çalıştıktan sonra başa döner.
    pub fn reset(&mut self) {
        self.current = self.initial;
    }
}

impl Default for Backoff {
    fn default() -> Self {
        Self::new(Duration::from_millis(500), Duration::from_secs(30))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn doubles_until_max_and_resets() {
        let mut b = Backoff::new(Duration::from_millis(100), Duration::from_millis(500));
        let got: Vec<u128> = (0..5).map(|_| b.next_delay().as_millis()).collect();
        assert_eq!(got, vec![100, 200, 400, 500, 500]);
        b.reset();
        assert_eq!(b.next_delay(), Duration::from_millis(100));
    }

    #[test]
    fn initial_is_capped_by_max() {
        let mut b = Backoff::new(Duration::from_secs(10), Duration::from_secs(1));
        assert_eq!(b.next_delay(), Duration::from_secs(1));
    }
}
