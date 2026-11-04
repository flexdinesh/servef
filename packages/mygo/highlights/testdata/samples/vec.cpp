#include <vector>
#include <string>

namespace demo {
template <typename T>
class Stack final {
public:
    void push(T&& v) { items_.emplace_back(std::move(v)); }
    [[nodiscard]] auto size() const noexcept -> std::size_t { return items_.size(); }
private:
    std::vector<T> items_;
};
}  // namespace demo
int main() { auto s = demo::Stack<std::string>{}; s.push(u8"x"); return 0x0; }
