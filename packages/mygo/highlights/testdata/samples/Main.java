package demo;

import java.util.List;

/** Javadoc */
@SuppressWarnings("unchecked")
public final class Main<T> implements Runnable {
    private static final int MAX = 10_000;
    public void run() {
        List<String> xs = List.of("a", "b");
        for (var x : xs) System.out.println(x + 'c' + 1.5f);
    }
}
